// Standalone wrapper: the panel is the whole page, so it opens immediately and
// cannot be dismissed (there is nothing behind it).
(function () {
  "use strict";

  function openNow() {
    if (window.__crushfling && typeof window.__crushfling.open === "function") {
      window.__crushfling.open();
    }
    var close = document.getElementById("cf-close");
    if (close && close.parentNode) close.parentNode.removeChild(close);
    document.title = "CrushFling Auto Register";
  }

  // The shared panel closes itself on Escape; in the standalone window that
  // would leave an empty grey screen, so swallow the key before it reaches it.
  document.addEventListener(
    "keydown",
    function (event) {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
      }
    },
    true,
  );

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", openNow);
  } else {
    openNow();
  }
})();
