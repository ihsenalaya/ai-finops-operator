# CRD — AIBreakEvenAnalysis

`aiops.imperium.io/v1alpha1`, namespaced. Short name : `aibreakeven`.

Compare le coût d'un modèle **API managée** au coût d'une alternative **auto-hébergée sur GPU** et
calcule le **point mort** (payback).

## Spec
| Champ | Type | Requis | Description |
|-------|------|:------:|-------------|
| `target.namespace` / `target.application` | string | | Workload analysé. |
| `currentModelRef` | string | ✔ | AIModel actuellement utilisé (API managée). |
| `alternativeSelfHosted.modelName` | string | ✔ | Modèle open-weights à auto-héberger. |
| `alternativeSelfHosted.runtime` | enum `vllm\|tgi\|ollama\|sglang` | ✔ (défaut `vllm`) | Runtime d'inférence OSS. |
| `alternativeSelfHosted.gpuType` / `gpuCount` | string / int32 | | SKU et nombre de GPU. |
| `alternativeSelfHosted.monthlyGpuCostEUR` | Quantity | ✔ | Coût GPU mensuel. |
| `alternativeSelfHosted.estimatedOpsCostEUR` | Quantity | | Coût ops/MLOps mensuel. |
| `alternativeSelfHosted.storageNetworkCostEUR` | Quantity | | Coût stockage/réseau mensuel. |
| `alternativeSelfHosted.migrationCostEUR` | Quantity | | Coût one-shot de migration. |
| `analysisWindowDays` | int32 (défaut 30) | | Fenêtre d'observation pour l'extrapolation. |

## Status
`managedMonthlyCostEUR`, `selfHostedMonthlyCostEUR`, `monthlySavingsEUR`, `paybackMonths` (string,
vide si pas d'économie), `recommendation` (`keep-managed\|investigate\|self-host`), `conditions[]`.

## Comportement du controller
Calcule le coût du modèle courant sur la fenêtre via [costengine](../features/costengine.md),
l'extrapole au mois, additionne les coûts auto-hébergés, applique
[breakevenengine](../features/breakevenengine.md), expose `ai_finops_breakeven_savings_eur`.

Formule (MVP) :
```
managed   = token_cost + provider_fixed
self      = gpu + ops + storage_network
savings   = managed - self
payback   = migration_cost / savings   (si savings > 0)
```
Reco : `savings<=0` → keep-managed ; `payback<=6 mois` → self-host ; sinon investigate.

## Exemple
[`..._aibreakevenanalysis.yaml`](../../config/samples/aiops_v1alpha1_aibreakevenanalysis.yaml).
