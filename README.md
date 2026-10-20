# k8s-practice

A small Go web app and PostgreSQL on a local [kind](https://kind.sigs.k8s.io/) cluster, with Traefik ingress, Prometheus/Grafana monitoring, and a GitOps CI/CD pipeline (GitHub Actions + Argo CD).

```
git push ──> GitHub Actions: vet, build, validate manifests
                 └─ main: push image to ghcr.io, bump tag in webapp-db/k8s/03-webapp.yaml
                                                   │
Argo CD (in the kind cluster) polls main every minute ─> syncs webapp-db/k8s to the cluster
```

## Layout

| Path | What |
|---|---|
| `webapp-db/app/` | Go web app: counts visits in PostgreSQL, exposes `/metrics`, `/healthz`, `/readyz` |
| `webapp-db/k8s/` | Namespace, Secret, PostgreSQL StatefulSet + PVC, web Deployment, Services, Ingress (synced by Argo CD) |
| `webapp-db/kind-config.yaml` | kind cluster with the ingress port mapped to `localhost:8080` |
| `webapp-db/traefik-values.yaml` | Traefik ingress controller |
| `monitoring/` | kube-prometheus-stack values, ServiceMonitor and alert rules for the app |
| `argocd/` | Argo CD values and the `webapp` Application |
| `.github/workflows/ci.yaml` | The pipeline |

## Set up the cluster from scratch

```sh
colima start --cpu 4 --memory 6
kind create cluster --name dev --config webapp-db/kind-config.yaml

helm repo add traefik https://traefik.github.io/charts
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/
helm repo add argo https://argoproj.github.io/argo-helm

helm upgrade --install traefik traefik/traefik -n traefik --create-namespace -f webapp-db/traefik-values.yaml
helm upgrade --install metrics-server metrics-server/metrics-server -n kube-system --set 'args={--kubelet-insecure-tls}'
helm upgrade --install monitoring prometheus-community/kube-prometheus-stack -n monitoring --create-namespace -f monitoring/kube-prometheus-stack-values.yaml
helm upgrade --install argocd argo/argo-cd -n argocd --create-namespace -f argocd/argocd-values.yaml

# Pull secret for the image on ghcr.io (not stored in Git)
kubectl create namespace webapp
kubectl -n webapp create secret docker-registry ghcr-pull \
  --docker-server=ghcr.io --docker-username=<github-user> --docker-password="$(gh auth token)"

kubectl apply -f argocd/webapp-application.yaml
kubectl apply -f monitoring/webapp-monitoring.yaml
```

## URLs

| | URL | Login |
|---|---|---|
| App | http://webapp.localhost:8080 | |
| Argo CD | http://argocd.localhost:8080 | `admin` / `kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' \| base64 -d` |
| Grafana | http://grafana.localhost:8080 | `admin` / `admin-test-password` |
| Prometheus | http://prometheus.localhost:8080 | |
| Alertmanager | http://alertmanager.localhost:8080 | |

The passwords in this repo are test-only values for a local cluster.
