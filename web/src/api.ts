export type Kind = "client" | "group" | "site";

export interface Role {
  node_id: number;
  role: "admin" | "editor";
  can_create: boolean;
  can_edit: boolean;
  can_publish: boolean;
  needs_approval: boolean;
}

export interface User {
  id: number;
  email: string;
  name: string;
  is_super: boolean;
  client_id: number | null;
  active: boolean;
  created_at: string;
  roles: Role[];
}

export interface TreeNode {
  id: number;
  parent_id: number | null;
  client_id: number;
  kind: Kind;
  name: string;
}

export interface Device {
  id: number;
  node_id: number | null;
  name: string;
  status: "pending" | "active" | "blocked";
  pairing_code?: string;
  auth_mode: "link" | "basic";
  auth_user: string;
  has_password: boolean;
  url?: string;
  user_agent: string;
  last_ip: string;
  last_seen_at: string | null;
  created_at: string;
  next_check_at: string | null;
  offline_since: string | null;
}

export interface Tree {
  nodes: TreeNode[];
  devices: Device[];
  server_time: string;
  hub_url?: string; // super admin only
}

export interface MediaWarning {
  level: "warn" | "error";
  code: string;
  message: string;
}

export interface Media {
  id: number;
  node_id: number;
  client_id: number;
  name: string;
  kind: "image" | "video" | "audio";
  sha256: string;
  ext: string;
  size: number;
  width: number;
  height: number;
  duration_ms: number;
  video_codec: string;
  audio_codec: string;
  fps: number;
  bitrate: number;
  status: "processing" | "ready" | "failed";
  error: string;
  has_thumb: boolean;
  created_at: string;
  warnings: MediaWarning[];
  owner_name: string;
  source_id: number | null;
  can_edit: boolean;
  convertible: boolean;
}

export interface CampaignItem {
  media_id: number;
  duration_s: number;
}

export interface CampaignSpec {
  items: CampaignItem[];
  audio_id?: number;
  fit: "meet" | "fill" | "slice";
  targets: number[];
  start: string;
  end: string;
  days: number[] | null;
  time_from: string;
  time_to: string;
  priority: number;
}

export type CampaignState = "draft" | "pending" | "active" | "scheduled" | "expired" | "stopped";

export interface CampaignVersion {
  id: number;
  number: number;
  status: "pending" | "approved" | "rejected";
  created_at: string;
  review_comment: string;
  spec: CampaignSpec;
  created_by_name: string;
  reviewed_by_name: string;
}

export interface CampaignSummary {
  id: number;
  name: string;
  node_id: number;
  owner_name: string;
  state: CampaignState;
  live: CampaignVersion | null;
  has_pending: boolean;
  draft_dirty: boolean;
  can_edit: boolean;
  can_publish: boolean;
  can_admin: boolean;
  needs_review: boolean;
  updated_at: string;
}

export interface CampaignDetail extends CampaignSummary {
  draft: CampaignSpec;
  versions: CampaignVersion[];
  ready: number;
  devices: number;
  late: number;
  latest_check?: string;
}

export interface PollPolicy {
  mode: "interval" | "times";
  minutes: number;
  times: string[] | null;
  spread_min: number;
}

export interface PollInfo {
  own: PollPolicy | null;
  effective: PollPolicy;
  description: string;
  from_node_id: number;
  from_name: string;
  can_edit: boolean;
}

export interface BackupResult {
  at: string;
  file?: string;
  size?: number;
  media_copied: number;
  error?: string;
}

export interface HubSettings {
  public_url: string;
  hub_url: string;
  hub_private: boolean;
  detected_urls: string[];
  hub_url_fixed: boolean;
  smtp_host: string;
  smtp_port: number;
  smtp_user: string;
  smtp_pass?: string;
  smtp_from: string;
  alert_emails: string[];
  alert_admins: boolean;
  alert_grace_min: number;
  backup_time: string;
  backup_keep: number;
  backup_dir: string;
  has_password: boolean;
  backup_path: string;
  last_backup: BackupResult;
  backups: { name: string; size: number; at: string }[];
  default_poll: PollPolicy;
}

export interface BillingRow {
  client_id: number;
  name: string;
  device_days: number;
  devices: number;
  active_end: number;
}

export interface Approval {
  campaign_id: number;
  campaign_name: string;
  node_id: number;
  owner_name: string;
  version_id: number;
  number: number;
  created_by_name: string;
  created_at: string;
}

export interface UploadResult {
  name: string;
  media?: Media;
  error?: string;
}

/** Uploads one file with progress; XHR because fetch has no upload progress. */
export function uploadFile(nodeId: number, file: File, onProgress: (fraction: number) => void): Promise<UploadResult> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `/api/media?node_id=${nodeId}`);
    xhr.setRequestHeader("X-Marquee", "1");
    xhr.upload.onprogress = (e) => e.lengthComputable && onProgress(e.loaded / e.total);
    xhr.onload = () => {
      if (xhr.status !== 200) {
        let msg = xhr.statusText;
        try {
          msg = JSON.parse(xhr.responseText).error ?? msg;
        } catch {
          /* not JSON */
        }
        reject(new ApiError(xhr.status, msg));
        return;
      }
      resolve((JSON.parse(xhr.responseText) as UploadResult[])[0] ?? { name: file.name, error: "empty response" });
    };
    xhr.onerror = () => reject(new ApiError(0, "Връзката прекъсна"));
    const fd = new FormData();
    fd.append("file", file, file.name);
    xhr.send(fd);
  });
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export async function api<T = void>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: { "Content-Type": "application/json", "X-Marquee": "1" },
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: "same-origin",
  });
  if (!res.ok) {
    let msg = res.statusText;
    try {
      msg = (await res.json()).error ?? msg;
    } catch {
      /* not JSON */
    }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

// Server messages are English; show the common ones in Bulgarian.
const messages: Record<string, string> = {
  "wrong e-mail or password": "Грешен имейл или парола",
  "too many failed attempts, try again later": "Твърде много грешни опити. Опитайте след 15 минути.",
  forbidden: "Нямате права за това действие",
  "already exists": "Вече съществува",
  "not empty: remove children and devices first": "Не е празно: първо премахнете подзвената и устройствата",
  "password must be at least 10 characters": "Паролата трябва да е поне 10 символа",
  "no pending device with this code": "Няма чакащо устройство с този код",
  "devices can only be placed on a site": "Устройство се добавя само към обект",
  "unsupported file type": "Неподдържан тип файл",
  "file too large": "Файлът е твърде голям",
  "ffmpeg is not available on the server": "На сървъра няма ffmpeg",
  "used in a campaign": "Файлът се използва в кампания",
  "the playlist is empty": "Плейлистът е празен",
  "choose where the campaign plays": "Изберете къде да се излъчва кампанията",
  "end must be after start": "Краят трябва да е след началото",
  "targets must be within the campaign's node": "Може да се излъчва само в звеното на кампанията и под него",
  "stop the campaign before deleting it": "Първо спрете кампанията",
  "version is not waiting for approval": "Тази версия вече е прегледана",
  "set both time_from and time_to, or neither": "Задайте и двата часа или нито един",
  "time_from and time_to must differ": "Часовете от и до трябва да са различни",
  "add at least one image": "Добавете поне една снимка",
  "every image must last longer than the transition": "Всяка снимка трябва да стои по-дълго от прехода",
  "image duration must be 1..120 s": "Снимка стои от 1 до 120 секунди",
  "the video may be at most 30 minutes": "Видеото може да е най-много 30 минути",
  "media not ready": "Файл още се обработва",
  "media from another branch": "Файл от друго звено не може да се ползва тук",
  "unknown media": "Файлът не е намерен",
  "text may be at most 300 characters": "Текстът може да е най-много 300 знака",
  "font not available on the server": "Шрифтът липсва на сървъра",
  "interval must be 1..1440 minutes": "Интервалът е от 1 до 1440 минути",
  "give 1..48 check times": "Въведете от 1 до 48 часа за проверка",
  "spread must be 0..120 minutes": "Разпределението е от 0 до 120 минути",
  "invalid SMTP port": "Невалиден SMTP порт",
  "invalid sender address": "Невалиден адрес на подателя",
  "grace must be 1..1440 minutes": "Толерансът е от 1 до 1440 минути",
  "invalid backup time": "Невалиден час за резервно копие",
  "keep 1..365 backups": "Пазят се от 1 до 365 копия",
  "e-mail is not configured": "Имейлът не е настроен",
  "month must be YYYY-MM": "Невалиден месец",
  "invalid hub address": "Невалиден адрес: въведете IP (напр. 85.10.20.30) или http://IP:порт",
};

export interface Slideshow {
  items: Slide[];
  audio_id?: number;
  transition: string;
  transition_s: number;
  motion: boolean;
  background: "blur" | "black" | "crop";
  portrait: boolean;
  text_style: TextStyle;
}

export interface Slide {
  media_id: number;
  duration_s: number;
  text?: string;
  text_pos?: "top" | "center" | "bottom";
}

export interface TextStyle {
  font: "arial" | "segoe" | "georgia" | "impact" | "verdana" | "comic";
  size: "s" | "m" | "l" | "xl";
  color: string;
  box: "none" | "dark" | "light";
}

export function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    if (messages[e.message]) return messages[e.message];
    const m = /^media "(.+)" is not (ready yet|available here)$/.exec(e.message);
    if (m) return m[2] === "ready yet" ? `„${m[1]}“ още се обработва` : `„${m[1]}“ не е достъпен за това звено`;
    return e.message;
  }
  return String(e);
}
