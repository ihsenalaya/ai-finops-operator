import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import Form from "@rjsf/core";
import validator from "@rjsf/validator-ajv8";
import type { RJSFSchema } from "@rjsf/utils";
import yaml from "js-yaml";
import { api } from "../api/client";

type Props = {
  kind: string;
  mode: "create" | "edit";
  namespace?: string;
  name?: string;
};

export default function ResourceForm({ kind, mode, namespace, name }: Props) {
  const navigate = useNavigate();
  const [schema, setSchema] = useState<RJSFSchema | null>(null);
  const [specData, setSpecData] = useState<Record<string, any>>({});
  const [metaName, setMetaName] = useState(name ?? "");
  const [metaNamespace, setMetaNamespace] = useState(namespace ?? "default");
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [showYAML, setShowYAML] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.listNamespaces().then(setNamespaces).catch(() => undefined);
  }, []);

  useEffect(() => {
    setLoading(true);
    setError(null);
    Promise.all([
      api.getSchema(kind),
      mode === "edit" && namespace && name ? api.getObject(kind, namespace, name) : Promise.resolve(null),
    ])
      .then(([fullSchema, existing]) => {
        const specSchema = (fullSchema as any)?.properties?.spec ?? {};
        setSchema(specSchema as RJSFSchema);
        if (existing) {
          setSpecData(existing.spec ?? {});
          setMetaName(existing.metadata.name);
          setMetaNamespace(existing.metadata.namespace);
        }
      })
      .catch((e) => setError(String(e)))
      .finally(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kind, mode, namespace, name]);

  const yamlPreview = useMemo(() => {
    const obj = {
      apiVersion: "aiops.imperium.io/v1alpha1",
      kind,
      metadata: { name: metaName, namespace: metaNamespace },
      spec: specData,
    };
    return yaml.dump(obj, { noRefs: true });
  }, [kind, metaName, metaNamespace, specData]);

  async function handleSubmit() {
    setError(null);
    setSaving(true);
    try {
      const body = {
        metadata: { name: metaName, namespace: metaNamespace },
        spec: specData,
      };
      if (mode === "create") {
        await api.createObject(kind, body);
      } else {
        await api.updateObject(kind, metaNamespace, metaName, body);
      }
      navigate(`/${kind}`);
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <p>Chargement…</p>;

  return (
    <div>
      <div className="toolbar">
        <h2>
          {mode === "create" ? "Nouveau " : "Modifier "}
          {kind}
        </h2>
        <div className="spacer" />
        <button className="button" onClick={() => setShowYAML((v) => !v)}>
          {showYAML ? "Voir le formulaire" : "Voir en YAML"}
        </button>
      </div>

      {error && <p className="error">{error}</p>}

      {showYAML ? (
        <pre className="yaml-preview">{yamlPreview}</pre>
      ) : (
        <>
          <fieldset className="metadata-fields">
            <legend>Métadonnées</legend>
            <label>
              Nom
              <input
                value={metaName}
                disabled={mode === "edit"}
                onChange={(e) => setMetaName(e.target.value)}
                placeholder="mon-objet"
              />
            </label>
            <label>
              Namespace
              <input
                list="namespace-options"
                value={metaNamespace}
                disabled={mode === "edit"}
                onChange={(e) => setMetaNamespace(e.target.value)}
              />
              <datalist id="namespace-options">
                {namespaces.map((ns) => (
                  <option key={ns} value={ns} />
                ))}
              </datalist>
            </label>
          </fieldset>

          {schema && (
            <Form
              schema={schema}
              formData={specData}
              validator={validator}
              onChange={(e) => setSpecData(e.formData ?? {})}
              onSubmit={handleSubmit}
            >
              <div className="form-actions">
                <button type="submit" className="button primary" disabled={saving || !metaName || !metaNamespace}>
                  {saving ? "Enregistrement…" : mode === "create" ? "Créer" : "Enregistrer"}
                </button>
              </div>
            </Form>
          )}
        </>
      )}
    </div>
  );
}
