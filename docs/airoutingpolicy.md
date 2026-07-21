# AIRoutingPolicy

`AIRoutingPolicy` (shortName `airpolicy`) déclare la politique d'**optimisation continue du
routage** inter-modèles : l'objectif principal, les garde-fous que tout candidat doit franchir,
la phase canary optionnelle, et — pour le plan GOV-AR — la politique typée de réservation /
calibration / drift / risque.

## Spec

| Champ | Type | Rôle |
|---|---|---|
| `objective` | enum `cost` \| `quality` \| `latency` | Dimension d'optimisation principale du moteur de routage. |
| `guardrails.minQualityScore` | number 0–1 | Score de routage minimal d'un candidat avant d'être proposé. |
| `guardrails.maxLatencyMillis` | int | Latence moyenne mesurée maximale acceptée pour un candidat. |
| `guardrails.requireSovereigntyCompliance` | bool | Restreint les candidats aux modèles conformes aux politiques de souveraineté. |
| `canary.enabled` | bool | Exige une validation canary avant reroute complet (actuation via `AIChangeRequest`). |
| `canary.percent` | int 1–50 | Pourcentage de trafic envoyé au candidat pendant le canary. |
| `spec.govar` | object | Politique GOV-AR typée (réservation, calibration, drift, cohorte, risque). Documentée en détail dans l'opérateur GOV-AR (`govar-safety-fields.md`). |

### Bloc `govar` (résumé)

- `reservation.method` (requis) : `strict_provider_cap`, `mean`, `fixed_margin`,
  `fixed_quantile`, `adaptive_quantile` ou `govar_fixed_cohort` — méthode exécutable de
  réservation de tokens de sortie.
- `calibration` (requis pour les méthodes adaptatives) : référence d'**artefact immuable**
  (`artifactRef`, `artifactSHA256`), digests d'entrées (`calibrationInputSHA256`,
  `capRegimeSHA256`, `priceRegimeSHA256`), cible de couverture en ppb, support minimal,
  fraîcheur maximale et version du producteur.
- `drift` (requis) : détecteur prédéclaré (`coverage-gap`, `psi`, `ks`, `adwin`), seuil en ppb,
  repli conservateur (`strict_provider_cap`, `queue`, `reject`, `abstain`, `require_approval`).
- `cohort` + `risk` (requis pour `govar_fixed_cohort`) : snapshot figé du registre de cohorte et
  budget de risque tenant en parties par milliard.

La validation CEL rejette une méthode sans ses entrées, une calibration adaptative absente, ou
une méthode fixed-cohort sans configuration cohorte/risque.

## Status

| Champ | Rôle |
|---|---|
| `recommendations[]` | Candidats évalués par le moteur : `currentModel`, `candidateModel`, `candidateScore`, `estimatedSavingsEUR`, `blocked` + `blockReason`. |
| `govar.calibration` | **Évidence produite par le controller** : estimation validée, support, couverture empirique/bornes (ppb), fenêtres, digests de régimes, `valid`. Renseigner la spec ne rend pas la calibration valide. |
| `govar.drift` | État visible du détecteur : `detected`, `conservativeMode`, couverture empirique, fenêtre de monitoring. |
| `lastEvaluatedAt`, `observedGeneration`, `conditions` | Réconciliation standard. |

## Exemple

```yaml
apiVersion: aiops.imperium.io/v1alpha1
kind: AIRoutingPolicy
metadata:
  name: cost-first
spec:
  objective: cost
  guardrails:
    minQualityScore: 0.7
    maxLatencyMillis: 1500
    requireSovereigntyCompliance: true
  canary:
    enabled: true
    percent: 10
```

## kubectl

```bash
kubectl get airpolicy
kubectl get airoutingpolicy cost-first -o jsonpath='{.status.recommendations}'
```
