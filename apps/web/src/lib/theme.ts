export type Theme = "dark" | "light";

let currentTheme: Theme = "light";
const listeners = new Set<(theme: Theme) => void>();

export function getTheme(): Theme {
  if (typeof window !== "undefined") {
    const saved = localStorage.getItem("kinetiq_theme") as Theme | null;
    if (saved === "light" || saved === "dark") {
      return saved;
    }
  }
  return "light";
}

export function setTheme(theme: Theme) {
  currentTheme = theme;
  if (typeof window !== "undefined") {
    localStorage.setItem("kinetiq_theme", theme);
    const root = document.documentElement;
    if (theme === "dark") {
      root.classList.add("dark");
      root.classList.remove("light");
    } else {
      root.classList.add("light");
      root.classList.remove("dark");
    }
  }
  listeners.forEach((fn) => fn(theme));
}

export function toggleTheme(): Theme {
  const next = getTheme() === "dark" ? "light" : "dark";
  setTheme(next);
  return next;
}

export function subscribeTheme(fn: (theme: Theme) => void) {
  listeners.add(fn);
  fn(getTheme());
  return () => {
    listeners.delete(fn);
  };
}
