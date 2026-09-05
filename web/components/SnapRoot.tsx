"use client";

import { useEffect } from "react";

/**
 * Scopes scroll snapping to the landing page: while mounted it stamps
 * data-snap on <html>, which activates the mandatory y-snap defined in
 * globals.css. The console never snaps.
 */
export default function SnapRoot() {
  useEffect(() => {
    document.documentElement.setAttribute("data-snap", "");
    return () => document.documentElement.removeAttribute("data-snap");
  }, []);
  return null;
}
