"use client";

import { useEffect } from "react";

/**
 * IntersectionObserver-driven reveals, following the house pattern from
 * THASSA's site: elements tagged .fx / .fx-scale transition in the first time
 * they enter the viewport. No animation library.
 */
export default function ScrollFX() {
  useEffect(() => {
    const els = document.querySelectorAll(".fx, .fx-scale");
    if (!("IntersectionObserver" in window)) {
      // Without the observer, show everything rather than leaving the page
      // permanently blank.
      els.forEach((el) => el.classList.add("in"));
      return;
    }
    const io = new IntersectionObserver(
      (entries) => {
        entries.forEach((e) => {
          if (e.isIntersecting) {
            e.target.classList.add("in");
            io.unobserve(e.target);
          }
        });
      },
      { rootMargin: "0px 0px -8% 0px", threshold: 0.1 },
    );
    els.forEach((el) => io.observe(el));
    return () => io.disconnect();
  }, []);
  return null;
}
