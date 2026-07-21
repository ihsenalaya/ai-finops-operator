# CRD — AIGateway

`aiops.imperium.io/v1alpha1`, namespaced. Short name : `aigw`.

Déclare une gateway IA existante (LiteLLM, Envoy, Kong, Gateway API, custom) que l'opérateur
observe en **lecture seule**, et indique comment collecter la télémétrie + quels namespaces sont gouvernés.

## Spec
| Champ | Type | Requis | Description |
|-------|------|:------:|-------------|
| `type` | enum `litellm\|envoy\|kong\|gateway-api\|custom` | ✔ | Technologie de la gateway. |
| `endpoint` | string | ✔ | URL de base de la gateway. |
| `namespaceSelector` | LabelSelector | | Namespaces gouvernés. `nil` ⇒ aucun (défaut sûr). |
| `telemetry.mode` | enum `prometheus\|aigw\|configmap\|fake` | ✔ (défaut `fake`) | Implémentation de collecte. `aigw` = Envoy AI Gateway/OTel (réel) ; `fake` = opt-in démo (pas de repli silencieux). Mode `litellm` retiré. |
| `telemetry.metricsEndpoint` | string | | Chemin métriques (modes `prometheus`/`aigw`), défaut `/metrics`. |
| `telemetry.sourceConfigMap` | string | | Nom de la `ConfigMap` (mode `configmap`, clé `usage.json`). |
| `auth.secretRef.name` | string | | Secret portant le token d'admin de la gateway. |
| `auth.secretRef.key` | string | | Clé dans le Secret. |

## Status
| Champ | Description |
|-------|-------------|
| `observedGeneration` | Génération réconciliée. |
| `governedNamespaces[]` | Namespaces résolus via `namespaceSelector`. |
| `conditions[]` | Condition standard `Ready`. |

## Comportement du controller
Résout `governedNamespaces` en listant les namespaces correspondant au sélecteur (RBAC lecture
namespaces), pose `Ready=True`, émet un Event `Reconciled`. Ne modifie jamais la gateway.

## Exemple
Voir [`config/samples/aiops_v1alpha1_aigateway.yaml`](../../config/samples/aiops_v1alpha1_aigateway.yaml).

```bash
kubectl get aigw
kubectl get aigw main-gateway -o jsonpath='{.status.governedNamespaces}'
```

Lié : [collectors](../features/collectors.md), [AIFinOpsReport](aifinopsreport.md).
