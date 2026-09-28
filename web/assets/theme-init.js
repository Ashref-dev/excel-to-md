(() => {
  const key = "theme-preference";

  const setTheme = (theme, source) => {
    document.documentElement.setAttribute("data-theme", theme);
    document.documentElement.setAttribute("data-theme-source", source);
  };

  try {
    const saved = window.localStorage.getItem(key);
    if (saved === "light" || saved === "dark") {
      setTheme(saved, "manual");
      return;
    }
  } catch (_) {}

  const isDark =
    typeof window.matchMedia === "function" && window.matchMedia("(prefers-color-scheme: dark)").matches;
  setTheme(isDark ? "dark" : "light", "system");
})();
