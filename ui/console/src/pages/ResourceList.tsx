import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";

export default function ResourceList({ kind }: { kind: string }) {
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [namespace, setNamespace] = useState<string>("");
  const [items, setItems] = useState<Record<string, any>[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.listNamespaces().then(setNamespaces).catch(() => undefined);
  }, []);

  useEffect(() => {
    setLoading(true);
    setError(null);
    api
      .listObjects(kind, namespace || undefined)
      .then(setItems)
      .catch((e) => setError(String(e)))
      .finally(() => setLoading(false));
  }, [kind, namespace]);

  async function handleDelete(ns: string, name: string) {
    if (!confirm(`Supprimer ${kind} "${name}" dans ${ns} ?`)) return;
    await api.deleteObject(kind, ns, name);
    setItems((prev) => prev.filter((i) => !(i.metadata.namespace === ns && i.metadata.name === name)));
  }

  return (
    <div>
      <div className="toolbar">
        <h2>{kind}</h2>
        <div className="spacer" />
        <select value={namespace} onChange={(e) => setNamespace(e.target.value)}>
          <option value="">Tous les namespaces</option>
          {namespaces.map((ns) => (
            <option key={ns} value={ns}>
              {ns}
            </option>
          ))}
        </select>
        <Link to={`/${kind}/new`} className="button primary">
          + Nouveau
        </Link>
      </div>

      {error && <p className="error">{error}</p>}
      {loading ? (
        <p>Chargement…</p>
      ) : items.length === 0 ? (
        <p className="empty">Aucun objet {kind} pour l'instant.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Nom</th>
              <th>Namespace</th>
              <th>Statut</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={`${item.metadata.namespace}/${item.metadata.name}`}>
                <td>
                  <Link to={`/${kind}/${item.metadata.namespace}/${item.metadata.name}`}>{item.metadata.name}</Link>
                </td>
                <td>{item.metadata.namespace}</td>
                <td>{readyStatus(item)}</td>
                <td className="row-actions">
                  <button className="link-danger" onClick={() => handleDelete(item.metadata.namespace, item.metadata.name)}>
                    Supprimer
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

// readyStatus reads the standard Ready condition off status.conditions, when
// the CRD's controller populates one — purely informative, falls back to a
// neutral dash when there is none (e.g. a policy with no background reconciler).
function readyStatus(item: Record<string, any>): string {
  const conditions = item.status?.conditions as Array<{ type: string; status: string }> | undefined;
  const ready = conditions?.find((c) => c.type === "Ready");
  if (!ready) return "—";
  return ready.status === "True" ? "✓ Ready" : "✗ Not Ready";
}
