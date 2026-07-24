/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package console

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Server exposes the generic CRD console REST API.
type Server struct {
	dyn   dynamic.Interface
	mux   *http.ServeMux
	stat  StaticFS
	regs  []Resource
	group string
}

// StaticFS abstracts serving the built frontend, so tests can run without it.
type StaticFS interface {
	http.Handler
}

// NewServer builds the console API server. dyn is a dynamic Kubernetes client
// built from the caller's own kubeconfig (see cmd/console-api/main.go) — this
// package never manages credentials itself.
func NewServer(dyn dynamic.Interface, ui StaticFS) *Server {
	s := &Server{dyn: dyn, mux: http.NewServeMux(), stat: ui, regs: Registry, group: apiGroup}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/resources", s.handleListResourceTypes)
	s.mux.HandleFunc("GET /api/namespaces", s.handleListNamespaces)
	s.mux.HandleFunc("GET /api/resources/{kind}/schema", s.handleSchema)
	s.mux.HandleFunc("GET /api/resources/{kind}", s.handleList)
	s.mux.HandleFunc("POST /api/resources/{kind}", s.handleCreate)
	s.mux.HandleFunc("GET /api/resources/{kind}/{namespace}/{name}", s.handleGet)
	s.mux.HandleFunc("PUT /api/resources/{kind}/{namespace}/{name}", s.handleUpdate)
	s.mux.HandleFunc("DELETE /api/resources/{kind}/{namespace}/{name}", s.handleDelete)
	if s.stat != nil {
		s.mux.Handle("/", s.stat)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleListResourceTypes lists which CRDs this console knows about, for the
// UI's navigation sidebar.
func (s *Server) handleListResourceTypes(w http.ResponseWriter, r *http.Request) {
	type typeInfo struct {
		Kind      string `json:"kind"`
		Plural    string `json:"plural"`
		ShortName string `json:"shortName"`
		Group     string `json:"group"`
		Version   string `json:"version"`
	}
	out := make([]typeInfo, 0, len(s.regs))
	for _, res := range s.regs {
		out = append(out, typeInfo{Kind: res.Kind, Plural: res.Plural, ShortName: res.ShortName, Group: res.Group, Version: res.Version})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	writeJSON(w, http.StatusOK, out)
}

// handleListNamespaces lists cluster namespaces for the UI's namespace picker.
func (s *Server) handleListNamespaces(w http.ResponseWriter, r *http.Request) {
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	list, err := s.dyn.Resource(gvr).List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, err)
		return
	}
	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		names = append(names, item.GetName())
	}
	sort.Strings(names)
	writeJSON(w, http.StatusOK, names)
}

// handleSchema returns the OpenAPI v3 schema for one CRD kind, read directly
// from the CustomResourceDefinition object installed on the cluster — so the
// form the UI renders always matches whatever version of the CRD is actually
// deployed, not a copy baked into the frontend at build time.
func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	res, ok := s.resourceForKind(w, r)
	if !ok {
		return
	}
	crdGVR := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	crd, err := s.dyn.Resource(crdGVR).Get(r.Context(), res.CRDName(), metav1.GetOptions{})
	if err != nil {
		writeError(w, fmt.Errorf("reading CRD %q (is it installed on this cluster?): %w", res.CRDName(), err))
		return
	}
	versions, found, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
	if err != nil || !found {
		writeError(w, fmt.Errorf("CRD %q has no spec.versions", res.CRDName()))
		return
	}
	for _, v := range versions {
		vm, ok := v.(map[string]interface{})
		if !ok || vm["name"] != res.Version {
			continue
		}
		schemaObj, found, err := unstructured.NestedMap(vm, "schema", "openAPIV3Schema")
		if err != nil || !found {
			writeError(w, fmt.Errorf("CRD %q version %q has no openAPIV3Schema", res.CRDName(), res.Version))
			return
		}
		writeJSON(w, http.StatusOK, schemaObj)
		return
	}
	writeError(w, fmt.Errorf("CRD %q has no version %q", res.CRDName(), res.Version))
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	res, ok := s.resourceForKind(w, r)
	if !ok {
		return
	}
	ns := r.URL.Query().Get("namespace")
	var list *unstructured.UnstructuredList
	var err error
	if ns != "" {
		list, err = s.dyn.Resource(res.GVR()).Namespace(ns).List(r.Context(), metav1.ListOptions{})
	} else {
		list, err = s.dyn.Resource(res.GVR()).Namespace(metav1.NamespaceAll).List(r.Context(), metav1.ListOptions{})
	}
	if err != nil {
		writeError(w, err)
		return
	}
	// unstructured.UnstructuredList keeps its items in a separate .Items field,
	// not inside .Object — encoding .Object directly silently drops them.
	// Encoding the list itself uses its MarshalJSON, which merges both.
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	res, ok := s.resourceForKind(w, r)
	if !ok {
		return
	}
	obj, err := s.dyn.Resource(res.GVR()).Namespace(r.PathValue("namespace")).Get(r.Context(), r.PathValue("name"), metav1.GetOptions{})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, obj.Object)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	res, ok := s.resourceForKind(w, r)
	if !ok {
		return
	}
	obj, err := decodeBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	obj.SetAPIVersion(res.Group + "/" + res.Version)
	obj.SetKind(res.Kind)
	ns := obj.GetNamespace()
	if ns == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "metadata.namespace is required"})
		return
	}
	created, err := s.dyn.Resource(res.GVR()).Namespace(ns).Create(r.Context(), obj, metav1.CreateOptions{})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created.Object)
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	res, ok := s.resourceForKind(w, r)
	if !ok {
		return
	}
	obj, err := decodeBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	obj.SetAPIVersion(res.Group + "/" + res.Version)
	obj.SetKind(res.Kind)
	obj.SetNamespace(ns)
	obj.SetName(name)
	if obj.GetResourceVersion() == "" {
		// Fetch the current resourceVersion so the update doesn't blindly clobber
		// a concurrent change — a UI must round-trip what it last read.
		current, err := s.dyn.Resource(res.GVR()).Namespace(ns).Get(r.Context(), name, metav1.GetOptions{})
		if err != nil {
			writeError(w, err)
			return
		}
		obj.SetResourceVersion(current.GetResourceVersion())
	}
	updated, err := s.dyn.Resource(res.GVR()).Namespace(ns).Update(r.Context(), obj, metav1.UpdateOptions{})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated.Object)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	res, ok := s.resourceForKind(w, r)
	if !ok {
		return
	}
	ns, name := r.PathValue("namespace"), r.PathValue("name")
	if err := s.dyn.Resource(res.GVR()).Namespace(ns).Delete(r.Context(), name, metav1.DeleteOptions{}); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resourceForKind(w http.ResponseWriter, r *http.Request) (Resource, bool) {
	kind := r.PathValue("kind")
	res, ok := ByKind(kind)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("unknown resource kind %q", kind)})
		return Resource{}, false
	}
	return res, true
}

func decodeBody(r *http.Request) (*unstructured.Unstructured, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	return &unstructured.Unstructured{Object: m}, nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if apierrors.IsNotFound(err) {
		status = http.StatusNotFound
	} else if apierrors.IsAlreadyExists(err) {
		status = http.StatusConflict
	} else if apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) {
		status = http.StatusBadRequest
	} else if apierrors.IsForbidden(err) {
		status = http.StatusForbidden
	} else if apierrors.IsUnauthorized(err) {
		status = http.StatusUnauthorized
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// CORS wraps a handler with permissive CORS headers, used only when the
// frontend dev server (Vite) runs on a different port than the API during
// local development. The production binary serves the built UI from the same
// origin/port as the API, so CORS does not apply there.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
