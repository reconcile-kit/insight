// The applied search state lives in the URL so views can be shared and survive reloads.

export const PAGE_SIZES = [50, 100, 200, 500];

export const emptyState = {
  name: "",
  resource_group: "",
  kind: "",
  namespace: "",
  shard_id: "",
  labels: "",
  pending: false,
  deleting: false,
  json: [],
  json_match: "all",
  sort: "",
  desc: false,
  page: 1,
  size: 100,
};

const TEXT = ["name", "resource_group", "kind", "namespace", "shard_id", "labels", "sort"];

export function readUrl() {
  const p = new URLSearchParams(window.location.search);
  const s = { ...emptyState };
  for (const k of TEXT) s[k] = p.get(k) || "";
  s.pending = p.get("pending") === "1";
  s.deleting = p.get("deleting") === "1";
  s.desc = p.get("desc") === "1";
  s.json_match = p.get("match") === "any" ? "any" : "all";
  s.page = Math.max(1, parseInt(p.get("page") || "1", 10) || 1);
  const size = parseInt(p.get("size") || "", 10);
  s.size = PAGE_SIZES.includes(size) ? size : emptyState.size;
  try {
    const j = JSON.parse(p.get("json") || "[]");
    if (Array.isArray(j)) s.json = j.map((r) => ({ ...newJsonRow(), ...r }));
  } catch {
    /* ignore broken links */
  }
  return s;
}

export function writeUrl(s) {
  const p = new URLSearchParams();
  for (const k of TEXT) if (s[k]) p.set(k, s[k]);
  if (s.pending) p.set("pending", "1");
  if (s.deleting) p.set("deleting", "1");
  if (s.desc) p.set("desc", "1");
  if (s.json_match === "any") p.set("match", "any");
  if (s.page > 1) p.set("page", String(s.page));
  if (s.size !== emptyState.size) p.set("size", String(s.size));
  const json = s.json.filter(isJsonRowActive).map(({ root, path, op, value }) => ({ root, path, op, value }));
  if (json.length) p.set("json", JSON.stringify(json));
  const qs = p.toString();
  const url = window.location.pathname + (qs ? `?${qs}` : "");
  if (url !== window.location.pathname + window.location.search) window.history.replaceState(null, "", url);
}

let rowId = 0;
export const newJsonRow = () => ({ id: ++rowId, root: "status", path: "", op: "eq", value: "" });

export const JSON_ROOTS = ["spec", "status", "annotations", "finalizers"];

export const JSON_OPS = [
  { op: "eq", label: "=", hint: "equals" },
  { op: "ne", label: "≠", hint: "not equal (field present)" },
  { op: "gt", label: ">", hint: "greater than" },
  { op: "gte", label: "≥", hint: "greater or equal" },
  { op: "lt", label: "<", hint: "less than" },
  { op: "lte", label: "≤", hint: "less or equal" },
  { op: "contains", label: "contains", hint: "contains substring" },
  { op: "not_contains", label: "!contains", hint: "does not contain" },
  { op: "matches", label: "regex", hint: "regular expression (case-insensitive)" },
  { op: "in", label: "in", hint: "one of comma-separated values" },
  { op: "exists", label: "exists", hint: "field exists" },
  { op: "not_exists", label: "!exists", hint: "field is missing" },
];

export const opNeedsValue = (op) => op !== "exists" && op !== "not_exists";

export const isJsonRowActive = (r) => !opNeedsValue(r.op) || String(r.value ?? "").trim() !== "";
