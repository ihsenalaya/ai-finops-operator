// Thin fetch wrapper around console-api's REST endpoints (internal/console).
// No SDK/codegen: the backend is intentionally generic (kind-agnostic), so
// every CRD this operator owns is reachable through the same few calls.

export type ResourceType = {
  kind: string;
  plural: string;
  shortName: string;
  group: string;
  version: string;
};

async function asJSON<T>(r: Response): Promise<T> {
  if (!r.ok) {
    let message = r.statusText;
    try {
      const body = await r.json();
      message = body.error ?? message;
    } catch {
      /* body wasn't JSON, keep statusText */
    }
    throw new Error(message);
  }
  return (await r.json()) as T;
}

export const api = {
  listResourceTypes: (): Promise<ResourceType[]> =>
    fetch("/api/resources").then((r) => asJSON<ResourceType[]>(r)),

  listNamespaces: (): Promise<string[]> => fetch("/api/namespaces").then((r) => asJSON<string[]>(r)),

  getSchema: (kind: string): Promise<Record<string, unknown>> =>
    fetch(`/api/resources/${kind}/schema`).then((r) => asJSON<Record<string, unknown>>(r)),

  listObjects: async (kind: string, namespace?: string): Promise<Record<string, any>[]> => {
    const qs = namespace ? `?namespace=${encodeURIComponent(namespace)}` : "";
    const data = await fetch(`/api/resources/${kind}${qs}`).then((r) => asJSON<any>(r));
    return data.items ?? [];
  },

  getObject: (kind: string, namespace: string, name: string): Promise<Record<string, any>> =>
    fetch(`/api/resources/${kind}/${namespace}/${name}`).then((r) => asJSON<Record<string, any>>(r)),

  createObject: (kind: string, body: Record<string, any>): Promise<Record<string, any>> =>
    fetch(`/api/resources/${kind}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then((r) => asJSON<Record<string, any>>(r)),

  updateObject: (kind: string, namespace: string, name: string, body: Record<string, any>): Promise<Record<string, any>> =>
    fetch(`/api/resources/${kind}/${namespace}/${name}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then((r) => asJSON<Record<string, any>>(r)),

  deleteObject: async (kind: string, namespace: string, name: string): Promise<void> => {
    const r = await fetch(`/api/resources/${kind}/${namespace}/${name}`, { method: "DELETE" });
    if (!r.ok && r.status !== 204) {
      throw new Error(await r.text());
    }
  },
};
