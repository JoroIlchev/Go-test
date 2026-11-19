import { admin } from "./admin";
import { campaigns } from "./campaigns";
import { devices } from "./devices";
import { media } from "./media";
import { shell } from "./shell";

// Each file holds the texts of a part of the admin (see ../../i18n.ts).
export const en: Record<string, string> = { ...shell, ...campaigns, ...media, ...devices, ...admin };
