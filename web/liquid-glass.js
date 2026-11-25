(() => {
  "use strict";

  function startLiquidGlass() {
    const targets = document.querySelectorAll(".kg-liquid-glass");
    if (!targets.length) return;

    if (typeof window.liquidGL !== "function") {
      document.documentElement.classList.add("kg-liquid-fallback");
      return;
    }

    const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const lowPower = Number(navigator.hardwareConcurrency || 8) <= 4;

    try {
      window.__kentgramLiquidGlass = window.liquidGL({
        engine: "auto",
        snapshot: "body",
        target: ".kg-liquid-glass",
        resolution: lowPower ? 0.9 : 1.25,
        refraction: 0.018,
        aberration: lowPower ? 0 : 0.006,
        bevelDepth: 0.045,
        bevelWidth: 0.12,
        frost: 1.6,
        shadow: true,
        specular: !reducedMotion && !lowPower,
        reveal: "fade",
        tilt: false,
        interaction: "none",
        tint: "rgba(20, 15, 16, 0.20)",
        on: {
          init() {
            document.documentElement.classList.add("kg-liquid-ready");
            document.documentElement.classList.remove("kg-liquid-fallback");
          }
        }
      });
    } catch (error) {
      console.warn("kentGram Liquid Glass fallback:", error);
      document.documentElement.classList.add("kg-liquid-fallback");
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", startLiquidGlass, { once: true });
  } else {
    startLiquidGlass();
  }
})();
