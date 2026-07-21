# AIChangeRequest

`AIChangeRequest` (shortName `aicrq`) est la **demande de changement gouvernée** : un workflow
d'approbation humaine porté par une CRD. Deux actions existent :

- **`reroute`** — proposer un changement de routage (modèle source → cible) avec économie
  attendue, score qualité et niveau de risque ; un humain approuve/rejette ; le controller
  actue la route à l'approbation.
- **`authorize-gov-ar-route`** — autoriser une **route GOV-AR exacte** (opérateur GOV-AR) :
  l'approbation lie les UID/générations précis de la policy/du modèle/du provider et le digest
  du route-snapshot. Voir la documentation de l'opérateur GOV-AR pour le cycle complet.

## Spec

| Champ | Type | Rôle |
|---|---|---|
| `action` * | enum `reroute` \| `authorize-gov-ar-route` | Type de changement proposé. |
| `approval` | enum `Pending` \| `Approved` \| `Rejected` | Décision du reviewer humain (vide initialement). |
| `sourceModel` / `targetModel` | string | Modèles concernés (reroute). |
| `expectedSavingEUR`, `qualityScore`, `latencyImpact`, `riskLevel`, `reason` | string | Dossier de décision présenté au reviewer. |
| `expiresAfter` | duration (ex. `24h`) | Délai d'attente avant expiration automatique. |
| `requestedBy` | string | Identité authentifiée du demandeur — **tamponnée par le webhook**, non déclarative. |
| `proposalDigest` | string | Digest liant tous les champs de la proposition (anti-altération). |
| `govarRouteApproval` | object | Portée exacte pour `authorize-gov-ar-route` : `routingPolicy`/`model`/`provider` (name+uid+generation), `routeSnapshotDigest`, `scopeDigest`, `validUntil`. |
| `govarDecision` | object | Enregistrement immuable de la décision reviewer lié à l'identité API server (`reviewerIdentity`, `reviewerGroup`, `admissionUID`, `decisionDigest`…). |

\* requis.

## Status

| Champ | Rôle |
|---|---|
| `phase` | `Pending` → `Approved` → `Actuated` \| `Rejected` \| `Expired` \| `Failed`. |
| `approvedBy`, `approvedAt` | Reviewer authentifié vérifié par le controller. |
| `approvedScopeDigest`, `approvedDecisionDigest` | Digests recomputés côté controller (GOV-AR). |
| `actuatedRoutes[]`, `actuatedAt` | Routes gateway effectivement modifiées (reroute). |
| `expiresAt`, `conditions`, `message` | Cycle de vie standard. |

## Chaîne d'approbation fail-closed (GOV-AR)

Quand l'opérateur GOV-AR est installé, ses webhooks d'admission :

1. **tamponnent** l'identité authentifiée du demandeur (`requestedBy`) à la création ;
2. **valident** qu'une décision (`approval: Approved|Rejected`) vient d'un reviewer du groupe
   `aiops.imperium.io:gov-ar-approval-reviewers`, différent du demandeur (pas
   d'auto-approbation), et enregistrent `govarDecision` de façon immuable.

Sans l'opérateur GOV-AR, le workflow `reroute` reste utilisable ; l'action
`authorize-gov-ar-route` nécessite l'opérateur GOV-AR.

## Exemple (reroute)

```yaml
apiVersion: aiops.imperium.io/v1alpha1
kind: AIChangeRequest
metadata:
  name: swap-gpt4o-mistral
spec:
  action: reroute
  sourceModel: gpt-4o
  targetModel: mistral-large-latest
  expectedSavingEUR: "120.50"
  riskLevel: low
  reason: "Mistral-Large atteint le même score qualité pour 45% du coût"
  expiresAfter: 48h
```

Approbation par le reviewer :

```bash
kubectl patch aicrq swap-gpt4o-mistral --type merge -p '{"spec":{"approval":"Approved"}}'
kubectl get aicrq swap-gpt4o-mistral -o jsonpath='{.status.phase}'
```
