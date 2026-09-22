import { useSettings } from "@/store/settings";
import { sfx } from "@/audio/sfx";

/**
 * Mute toggle + volume slider; the slider previews the level on release.
 * `compact` drops the slider — a phone has no room for it, and the device's
 * own volume keys do the same job.
 */
export function SoundControl({ compact = false }: { compact?: boolean }) {
  const { sfx: on, volume, setSfx, setVolume } = useSettings();
  return (
    <span className="flex items-center gap-1.5">
      <button
        type="button"
        title={on ? "Mute sound effects" : "Unmute sound effects"}
        aria-label={on ? "Mute sound effects" : "Unmute sound effects"}
        aria-pressed={on}
        className={compact ? "px-1 py-0.5 hover:text-white" : "hover:text-white"}
        onClick={() => {
          setSfx(!on);
          if (!on) sfx.play("chime");
        }}
      >
        {on ? "🔊" : "🔇"}
      </button>
      {!compact && (
        <input
          type="range"
          min={0}
          max={1}
          step={0.05}
          value={volume}
          disabled={!on}
          aria-label="Sound effects volume"
          className="w-16 accent-emerald-400 disabled:opacity-40"
          onChange={(e) => setVolume(Number(e.target.value))}
          onPointerUp={() => sfx.play("cashIn")}
          onKeyUp={() => sfx.play("cashIn")}
        />
      )}
    </span>
  );
}
