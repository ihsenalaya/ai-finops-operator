# AIRouteOverride

`AIRouteOverride` (shortName `airoverride`) est le **reroute manuel immédiat** : il demande à
l'opérateur de router le trafic d'un modèle source vers un modèle cible dans la gateway
(Envoy AI Gateway), sans attendre le moteur d'optimisation. Les mutations sont réversibles —
supprimer l'override restaure la route d'origine.

## Spec

| Champ | Type | Rôle |
|---|---|---|
| `sourceModel` * | string | ID provider du modèle qui sert actuellement le trafic. |
| `targetModel` * | string | ID provider du modèle vers lequel router. |
| `reason` | string | Justification lisible (audit). |

\* requis.

## Status

| Champ | Rôle |
|---|---|
| `phase` | `Pending` → `Actuated` \| `Failed` \| `Reverted`. |
| `actuatedRoutes[]` | Noms des `AIGatewayRoute` réellement modifiées. |
| `message`, `conditions`, `observedGeneration` | Diagnostic standard. |

## Exemple

```yaml
apiVersion: aiops.imperium.io/v1alpha1
kind: AIRouteOverride
metadata:
  name: hotfix-gpt4o-to-mistral
spec:
  sourceModel: gpt-4o
  targetModel: mistral-large-latest
  reason: "Incident coût US — bascule immédiate vers le modèle EU"
```

## kubectl

```bash
kubectl get airoverride
kubectl get airouteoverride hotfix-gpt4o-to-mistral -o jsonpath='{.status.phase}'
# revert :
kubectl delete airouteoverride hotfix-gpt4o-to-mistral
```

> L'actuation nécessite une Envoy AI Gateway avec des `AIGatewayRoute`
> (`aigateway.envoyproxy.io`) accessibles à l'opérateur. Sans gateway, l'override reste
> `Pending` avec une condition explicative.
