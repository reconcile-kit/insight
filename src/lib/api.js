// Reads with complex filters go to the insight backend (straight to the DB replica).
// A single resource GET and all writes go to state-manager through the /api proxy.

async function request(url, init) {
  const res = await fetch(url, init);
  const text = await res.text();
  let body = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = text;
  }
  if (!res.ok) {
    const msg = (body && body.error) || (typeof body === "string" && body) || res.statusText;
    const err = new Error(`${res.status}: ${msg}`);
    err.status = res.status;
    throw err;
  }
  return body;
}

const post = (url, body, signal) =>
  request(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });

/** Converts UI state into the backend search request. */
export function toSearch(state, { page = true } = {}) {
  const s = {
    filters: {
      name: state.name || "",
      resource_group: state.resource_group || "",
      kind: state.kind || "",
      namespace: state.namespace || "",
      shard_id: state.shard_id || "",
      pending: !!state.pending,
      deleting: !!state.deleting,
    },
    labels: state.labels || "",
    json: (state.json || []).map(({ root, path, op, value }) => ({ root, path, op, value: value ?? "" })),
    json_match: state.json_match || "all",
    sort: { field: state.sort || "", desc: !!state.desc },
  };
  if (page) {
    s.limit = state.size;
    s.offset = (state.page - 1) * state.size;
  }
  return s;
}

export const searchResources = (state, signal) => post("/insight/v1/resources/search", toSearch(state), signal);

export const fetchValues = (field, q, state, signal) =>
  post("/insight/v1/values", { field, q, search: toSearch(state, { page: false }), limit: 200 }, signal).then(
    (r) => r.values
  );


const resourceUrl = ({ resource_group, namespace, kind, name }) =>
  `/api/v1/groups/${encodeURIComponent(resource_group)}/namespaces/${encodeURIComponent(namespace)}` +
  `/kinds/${encodeURIComponent(kind)}/resources/${name === undefined ? "" : encodeURIComponent(name)}`;

export const getResource = (id, signal) => request(resourceUrl(id), { signal });

export const createResource = (r) =>
  request(resourceUrl({ ...r, name: undefined }), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(r),
  });

export const updateResource = (id, r) =>
  request(resourceUrl(id), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(r),
  });

export const deleteResource = (id) => request(resourceUrl(id), { method: "DELETE" });
