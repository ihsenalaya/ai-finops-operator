import { useEffect, useState } from "react";
import { HashRouter, Routes, Route, Navigate, NavLink, useParams } from "react-router-dom";
import { api, ResourceType } from "./api/client";
import ResourceList from "./pages/ResourceList";
import ResourceForm from "./pages/ResourceForm";

const OPERATOR_NAME = "AI FinOps Operator";

export default function App() {
  const [types, setTypes] = useState<ResourceType[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .listResourceTypes()
      .then(setTypes)
      .catch((e) => setError(String(e)));
  }, []);

  return (
    <HashRouter>
      <div className="layout">
        <aside className="sidebar">
          <h1>{OPERATOR_NAME}</h1>
          <p className="subtitle">Console</p>
          {error && <p className="error">Impossible de contacter console-api : {error}</p>}
          <nav>
            {types?.map((t) => (
              <NavLink key={t.kind} to={`/${t.kind}`} className={({ isActive }) => (isActive ? "active" : "")}>
                {t.kind}
                <span className="shortname"> {t.shortName}</span>
              </NavLink>
            ))}
          </nav>
        </aside>
        <main className="content">
          <Routes>
            <Route path="/" element={types && types.length > 0 ? <Navigate to={`/${types[0].kind}`} replace /> : <p>Chargement…</p>} />
            <Route path="/:kind" element={<ResourceListRoute />} />
            <Route path="/:kind/new" element={<ResourceFormRoute mode="create" />} />
            <Route path="/:kind/:namespace/:name" element={<ResourceFormRoute mode="edit" />} />
          </Routes>
        </main>
      </div>
    </HashRouter>
  );
}

function ResourceListRoute() {
  const { kind } = useParams();
  if (!kind) return null;
  return <ResourceList kind={kind} />;
}

function ResourceFormRoute({ mode }: { mode: "create" | "edit" }) {
  const { kind, namespace, name } = useParams();
  if (!kind) return null;
  return <ResourceForm kind={kind} mode={mode} namespace={namespace} name={name} />;
}
