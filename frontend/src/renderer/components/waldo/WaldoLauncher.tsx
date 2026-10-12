import { useTranslation } from "react-i18next";
import waldoMark from "../../../../assets/waldo-mark.svg";
import { isMacPlatform } from "../../lib/platform";
import { cn } from "../../lib/utils";
import { currentShortcutBindings } from "../../stores/keybindings-store";
import { shortcutBindingLabel } from "../../../shared/shortcuts";
import { useWaldoRail } from "./WaldoRailContext";

export function WaldoLauncher({ className }: { className?: string }) {
  const { t } = useTranslation();
  const waldo = useWaldoRail();
  const shortcut = currentShortcutBindings("toggle-waldo")[0];
  const shortcutLabel = shortcut ? shortcutBindingLabel(shortcut, isMacPlatform()) : "";

  return (
    <button
      aria-controls="waldo-rail"
      aria-expanded={waldo.isOpen}
      aria-label={t("shortcut.toggle-waldo")}
      className={cn(
        "waldo-native-interactive grid size-8 place-items-center rounded-full hairline border-wd-hairline bg-wd-surface text-wd-secondary transition-colors duration-fast ease-out hover:bg-wd-hover hover:text-wd-primary active:bg-wd-pressed motion-reduce:transition-none",
        waldo.isOpen &&
          "border-transparent bg-wd-inverse text-wd-on-inverse hover:bg-wd-inverse hover:text-wd-on-inverse active:bg-wd-inverse",
        className,
      )}
      onClick={(event) => waldo.toggle(event.currentTarget)}
      ref={waldo.launcherRef}
      title={`${t("shortcut.toggle-waldo")} · ${shortcutLabel}`}
      type="button"
    >
      <img alt="" aria-hidden="true" className="size-4 object-contain" data-brand="waldo" src={waldoMark} />
    </button>
  );
}
