import { useMemo } from "react";
import { qrMatrix } from "./qr";

/**
 * Renders a value as a QR symbol. Always light-on-dark-modules with a quiet
 * zone, whatever the surrounding theme, because that is what scanners want.
 */
export function QrCode({ value, label, className = "" }: { value: string; label: string; className?: string }) {
  const matrix = useMemo(() => {
    try {
      return qrMatrix(value);
    } catch {
      return null; // too long to encode: the copyable link is still there
    }
  }, [value]);

  if (!matrix) return null;

  const quiet = 4; // the quiet zone the spec asks for
  const size = matrix.length + quiet * 2;
  let path = "";
  matrix.forEach((row, y) => {
    row.forEach((dark, x) => {
      if (dark) path += `M${x + quiet} ${y + quiet}h1v1h-1z`;
    });
  });

  return (
    <svg
      viewBox={`0 0 ${size} ${size}`}
      role="img"
      aria-label={label}
      shapeRendering="crispEdges"
      className={`bg-white rounded-lg ${className}`}
    >
      <path d={path} fill="#020617" />
    </svg>
  );
}
