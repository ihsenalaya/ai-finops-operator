# CRD — AIBudgetPolicy

`aiops.imperium.io/v1alpha1`, namespaced. Short name : `aibudget`.

Définit un budget IA pour un namespace/équipe/application, avec seuils, actions recommandées et
**fallback managé optionnel** réellement actué au gateway quand le budget entre en zone critique.

## Spec
| Champ | Type | Requis | Description |
|-------|------|:------:|-------------|
| `target.namespace` / `target.team` / `target.application` | string | | Portée du budget (filtres cumulatifs). |
| `period` | enum `daily\|weekly\|monthly` | ✔ (défaut `monthly`) | Fenêtre budgétaire. |
| `budgetEUR` | Quantity | ✔ | Plafond sur la période (EUR). |
| `warningThresholdPercent` | int32 (0–100, défaut 70) | | Seuil warning. |
| `criticalThresholdPercent` | int32 (0–100, défaut 90) | | Seuil critical. |
| `hardLimitPercent` | int32 (0–200, défaut 100) | | Seuil hard-limit. |
| `actions.onWarning/onCritical/onHardLimit[]` | []string | | Actions recommandées par palier. |
| `fallbackModelRef` | string | | `AIModel` managé cible pour le fallback live. |
| `enforcementMode` | enum `reportOnly\|warn\|enforce` (défaut `reportOnly`) | | `enforce` autorise l'actuation du fallback au gateway. |
| `fallbackOnPhase` | enum `Critical\|Exceeded` (défaut `Exceeded`) | | Phase minimale qui peut déclencher le fallback live. |
| `minFallbackQualityTier` | enum `low\|medium\|high` | | Garde-fou qualité minimal. |
| `maxFallbackLatencyMillis` | int32 | | Garde-fou de latence moyenne observée (si la télémétrie le permet). |
| `maxFallbackErrorPercent` | int32 | | Garde-fou de taux d'erreur observé (si la télémétrie le permet). |

## Status
`observedGeneration`, `currentSpendEUR`, `usagePercent`, `phase`
(`Unknown\|WithinBudget\|Warning\|Critical\|Exceeded`), `activeFallbackModel`,
`fallbackActuated`, `fallbackReason`, `conditions[]`.

## Comportement du controller
Collecte l'usage (collector de la gateway, sinon `fake`), calcule le coût de la cible via
[costengine](../features/costengine.md), évalue avec [budgetengine](../features/budgetengine.md),
écrit `phase`/`usagePercent`/`currentSpendEUR`, expose `ai_finops_budget_usage_percent`, et émet
un Event `Warning` listant les actions recommandées si un palier est franchi. Si `fallbackModelRef`
est défini et `enforcementMode=enforce`, le controller peut **rerouter réellement** au gateway les
modèles de la cible vers ce fallback, sous conditions :

- le fallback résout vers un `AIProvider.managed=true` ;
- le fallback est moins cher sur le mix de tokens observé ;
- le modèle courant n'est pas partagé hors de la cible ;
- aucune `AISovereigntyPolicy` en `enforce` n'est déjà active ;
- les garde-fous qualité/latence/erreur, s'ils sont définis, sont respectés.

Le reroute est réversible et sa cause apparaît dans `fallbackReason`.

## Exemple
[`..._aibudgetpolicy.yaml`](../../config/samples/aiops_v1alpha1_aibudgetpolicy.yaml).

```bash
kubectl get aibudget rh-budget   # colonnes BUDGET(EUR) USAGE% PHASE
```
