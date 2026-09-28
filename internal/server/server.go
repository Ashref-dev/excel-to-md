package server

import (
	"context"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"excel-to-md/internal/config"
	"excel-to-md/internal/convert"
	"excel-to-md/web"
)

var (
	requestsTotal    = expvar.NewInt("requests_total")
	conversionsTotal = expvar.NewInt("conversions_total")
	conversionErrors = expvar.NewInt("conversion_errors_total")
	sheetErrorsTotal = expvar.NewInt("sheet_errors_total")
)

type apiError struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

type apiResponse struct {
	OK bool `json:"ok"`
	convert.Result
}

// New returns the HTTP handler for the API and the embedded frontend.
func New(cfg config.Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/convert", convertHandler(cfg))
	mux.HandleFunc("/health", healthHandler)
	if cfg.EnableDebugVars {
		mux.Handle("/debug/vars", expvar.Handler())
	}

	staticHandler := web.Handler()
	mux.Handle("/", staticHandler)

	return loggingMiddleware(securityHeadersMiddleware(mux))
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func convertHandler(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestsTotal.Add(1)
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxUploadBytes+multipartOverhead)
		if err := r.ParseMultipartForm(cfg.MaxUploadBytes); err != nil {
			conversionErrors.Add(1)
			writeError(w, http.StatusBadRequest, "Unable to read upload. Make sure the file is under the size limit.")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			conversionErrors.Add(1)
			writeError(w, http.StatusBadRequest, "Missing file upload.")
			return
		}
		defer file.Close()

		if !supportedExtension(header.Filename) {
			conversionErrors.Add(1)
			writeError(w, http.StatusBadRequest, "Only .xlsx, .xls and .csv files are supported.")
			return
		}

		payload, err := readUpload(file, cfg.MaxUploadBytes)
		if err != nil {
			conversionErrors.Add(1)
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), cfg.ConversionTimeout)
		defer cancel()

		result, err := convert.Convert(ctx, payload, convert.Options{
			Filename:            header.Filename,
			IncludeHiddenSheets: cfg.IncludeHiddenSheets,
			MaxSheets:           cfg.MaxSheets,
			MaxCellsPerSheet:    cfg.MaxCellsPerSheet,
		})
		if err != nil {
			conversionErrors.Add(1)
			status := http.StatusBadRequest
			if errors.Is(err, convert.ErrConversionTimeout) {
				status = http.StatusRequestTimeout
			}
			writeError(w, status, err.Error())
			return
		}

		conversionsTotal.Add(1)
		for _, sheet := range result.Sheets {
			if sheet.Error != "" {
				sheetErrorsTotal.Add(1)
			}
		}

		writeJSON(w, http.StatusOK, apiResponse{OK: true, Result: result})
	}
}

// multipartOverhead lets a file of exactly MaxUploadBytes through; readUpload enforces the file limit itself.
const multipartOverhead = 64 << 10

func supportedExtension(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".xlsx", ".xlsm", ".xls", ".csv", ".tsv":
		return true
	}
	return false
}

func readUpload(file multipart.File, limit int64) ([]byte, error) {
	payload, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read upload")
	}
	if int64(len(payload)) > limit {
		return nil, fmt.Errorf("file exceeds maximum size")
	}
	return payload, nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, apiError{OK: false, Error: message})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		duration := time.Since(start)
		fmt.Printf("%s %s %d %s\n", r.Method, r.URL.Path, wrapped.status, duration)
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
