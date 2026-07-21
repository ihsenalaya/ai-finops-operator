# CRD — AIFinOpsReport

`aiops.imperium.io/v1alpha1`, namespaced. Short name : `aireport`.

Rapport FinOps & souveraineté généré par l'opérateur. Les agrégats vivent dans `.status` ; le
rapport complet (Markdown + JSON) est écrit dans un **ConfigMap** `<nom>-report`.

## Spec
| Champ | Type | Requis | Description |
|-------|------|:------:|-------------|
| `target.namespace` | string | | Namespace ciblé (filtre l'usage). |
| `target.period` | enum `daily\|weekly\|monthly` | ✔ (défaut `monthly`) | Période. |
| `gatewayRef` | string | | AIGateway dont on prend le mode de télémétrie. |

## Status
| Champ | Description |
|-------|-------------|
| `generatedAt` | Horodatage de génération. |
| `totalCostEUR`, `totalInputTokens`, `totalOutputTokens` | Totaux sur la période. |
| `topModels[]` | `{name, costEUR}` triés par coût. |
| `sovereigntyFindings[]` | `{severity, message, namespace, application, model, provider, zone, requests}` — vérification **par flux**, attribuée au namespace/app/modèle/fournisseur, avec le nombre de requêtes affectées. |
| `recommendations[]` | `{type, message}`. |
| `latencyTelemetryAvailable` | `true` si au moins un score utilise une latence réellement observée ; `false` sinon. |
| `routingScores[]` | Score par namespace/app/modèle : `score`, `costScore`, `qualityScore`, `latencyScore`, `reliabilityScore`, coût, latence observée si disponible, `latencyTelemetryAvailable`, `latencySource`. Les valeurs décimales du status sont sérialisées en chaînes pour compatibilité CRD ; les métriques Prometheus restent numériques. Le score existe pour tout usage observé ; la latence mesurée n'est jamais inventée. |
| `conditions[]` | `Ready` (`reason=ReportGenerated`). |

## Comportement du controller
1. Résout la gateway (`gatewayRef`) → choisit le collector ; collecte l'usage.
2. Filtre par `target.namespace`, calcule via [costengine](../features/costengine.md).
3. Évalue la souveraineté via [sovereigntyengine](../features/sovereigntyengine.md).
4. Calcule les scores runtime via tokens/coût/catalogue et latence observée si disponible.
5. Écrit le `.status` + un **ConfigMap** `<nom>-report` (clés `report.md`, `report.json`) en
   owner-reference (GC avec le rapport) — voir [reporting](../features/reporting.md).
6. Expose `ai_finops_cost_eur`, `ai_finops_requests`, `ai_finops_latency_score`, etc. (gauges, sans `_total`). Re-réconcilie toutes les 60 s.

## Exemple
[`..._aifinopsreport.yaml`](../../config/samples/aiops_v1alpha1_aifinopsreport.yaml).

```bash
kubectl get aireport monthly-ai-report-rh -o yaml | yq '.status'
kubectl get cm monthly-ai-report-rh-report -o jsonpath='{.data.report\.md}'
```
