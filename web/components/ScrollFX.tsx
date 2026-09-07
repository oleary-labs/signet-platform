"use client";

import { useEffect } from "react";

const SELECTOR = ".fx, .fx-scale";

/**
 * IntersectionObserver-driven reveals, following the house pattern from
 * THASSA's site: elements tagged .fx / .fx-scale transition in the first time
 * they enter the viewport. No animation library.
 */
export default function ScrollFX() {
  useEffect(() => {
    if (!("IntersectionObserver" in window)) {
      // Without the observer, show everything rather than leaving the page
      // permanently blank.
      document.querySelectorAll(SELECTOR).forEach((el) => el.classList.add("in"));
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

    // Observing an element twice is a no-op, so callers never have to track
    // what has already been seen.
    const observe = (root: Element | Document) => {
      if (root instanceof Element && root.matches(SELECTOR)) io.observe(root);
      root.querySelectorAll(SELECTOR).forEach((el) => io.observe(el));
    };

    observe(document);

    // Screens that fetch their own content render their .fx elements after
    // this effect has already run — the operator marketplace fills its grid
    // from a query, and its filters replace every card on each change. A
    // one-shot querySelectorAll would leave all of them at opacity 0 forever,
    // so watch for the ones that arrive later.
    const mo = new MutationObserver((records) => {
      records.forEach((record) => {
        record.addedNodes.forEach((node) => {
          if (node.nodeType === Node.ELEMENT_NODE) observe(node as Element);
        });
      });
    });
    mo.observe(document.body, { childList: true, subtree: true });

    return () => {
      mo.disconnect();
      io.disconnect();
    };
  }, []);
  return null;
}
