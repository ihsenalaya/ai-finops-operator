# CRD — AIModel

`aiops.imperium.io/v1alpha1`, namespaced.

Catalogue un modèle IA disponible et le lie à un [AIProvider](aiprovider.md).

## Spec
| Champ | Type | Requis | Description |
|-------|------|:------:|-------------|
| `providerRef` | string | ✔ | Nom de l'AIProvider (même namespace). |
| `modelName` | string | ✔ | Identifiant côté fournisseur (ex. `gpt-4o`). Clé du price-book. |
| `type` | enum `llm\|embedding\|reranker\|vision\|audio` | ✔ (défaut `llm`) | Catégorie. |
| `contextWindow` | int32 | | Taille de contexte (tokens). |
| `qualityTier` | enum `low\|medium\|high` | | Niveau de qualité. |
| `costTier` | enum `low\|medium\|high` | | Niveau de coût relatif. |
| `sensitiveDataAllowed` | bool | | Donnée sensible autorisée sur ce modèle. |

## Status
`observedGeneration`, `resolvedProvider` (type du provider résolu), `lastQualityScore`,
`lastEvaluatedAt`, `conditions[]` (`Ready`).

`lastQualityScore` et `lastEvaluatedAt` résument la dernière évaluation `AIQualityGate` qui ciblait
ce modèle. Le détail auditable reste porté par le `AIQualityGate.status` et son `evidenceRef`.

## Comportement du controller
Résout `providerRef` :
- **provider absent** ⇒ `Ready=False` (`reason=ReferenceNotFound`) ; pas d'erreur dure.
- **provider présent** ⇒ `resolvedProvider` renseigné, `Ready=True`.

Un **watch** sur AIProvider réenfile les AIModel concernés quand le provider apparaît (ordre de
création indifférent).

## Exemple
[`..._aimodel.yaml`](../../config/samples/aiops_v1alpha1_aimodel.yaml),
[`..._aimodel_gpt4o.yaml`](../../config/samples/aiops_v1alpha1_aimodel_gpt4o.yaml).

```bash
kubectl get aimodel mistral-small -o jsonpath='{.status.resolvedProvider}'
```
