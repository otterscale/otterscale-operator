# Otterscale Operator

[![GitHub Release](https://img.shields.io/github/v/release/otterscale/otterscale-operator?logo=github)](https://github.com/otterscale/otterscale-operator/releases)
[![GitHub License](https://img.shields.io/github/license/otterscale/otterscale-operator?logo=github)](https://opensource.org/license/apache-2-0)
[![Lint Workflow](https://img.shields.io/github/actions/workflow/status/otterscale/otterscale-operator/lint.yml?logo=github&label=lint)](https://github.com/otterscale/otterscale-operator/actions/workflows/lint.yml)
[![Test Workflow](https://img.shields.io/github/actions/workflow/status/otterscale/otterscale-operator/test.yml?logo=github&label=test)](https://github.com/otterscale/otterscale-operator/actions/workflows/test.yml)
[![Test E2E Workflow](https://img.shields.io/github/actions/workflow/status/otterscale/otterscale-operator/test-e2e.yml?logo=github&label=test%20e2e)](https://github.com/otterscale/otterscale-operator/actions/workflows/test-e2e.yml)
[![GitHub Registry](https://img.shields.io/github/actions/workflow/status/otterscale/otterscale-operator/ghcr.yml?logo=github&label=ghcr)](https://github.com/otterscale/otterscale-operator/actions/workflows/ghcr.yml)

[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/11794/badge)](https://www.bestpractices.dev/projects/11794)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/otterscale/otterscale-operator/badge)](https://scorecard.dev/viewer/?uri=github.com/otterscale/otterscale-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/otterscale/otterscale-operator)](https://goreportcard.com/report/github.com/otterscale/otterscale-operator)
[![Codecov](https://codecov.io/gh/otterscale/otterscale-operator/graph/badge.svg?token=2wcwTTorFq)](https://codecov.io/gh/otterscale/otterscale-operator)

// TODO(user): Add simple overview of use/purpose

## Description

// TODO(user): An in-depth paragraph about your project and overview of use

## Getting Started

### Prerequisites

- go version v1.25.6+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster

**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/otterscale-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/otterscale-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
> privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

> **NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall

**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/otterscale-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/otterscale-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
   can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing

// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026 The OtterScale Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

# TEMP

實體層: metal, bootstrap

核心層: fleet, compute, storage, network

連線層: link (負責 Tunnel/Agent) ⬅️ 新加入成員

維運層: ops, telemetry, protection

治理層: identity, governance, finops, audit

應用層: hub, workflow, database, inference, training

---

如果您採取 「Edge 為主，Core 為輔」 的策略（即：CRD 存 Edge，Core 只做 Read-Only Dashboard），這在業界稱為 「聯邦唯讀模式 (Federated Observability)」。

---

Core (Hub)：存放 「意圖 (Intent)」 與 「全域設定 (Global Config)」。
Edge (Agent)：存放 「執行實體 (Runtime)」 與 「本地資源 (Local Resource)」。

| Level  | Group      | Kind                 | Core | Edge |
| ------ | ---------- | -------------------- | :--: | :--: |
| 應用層 | inference  | Service, Model       |      |  v   |
|        | training   | FinetuneJob, Dataset |      |  v   |
|        | hub        | Release, Chart       |      |  v   |
|        | database   | Redis, Postgres      |      |  v   |
|        | workflow   | Pipeline, Task       |      |  v   |
| 治理層 | identity   | Workspace, User      |      |  v   |
|        | governance | Policy, Compliance   |      |  v   |
|        | finops     | Budget, CostReport   |      |  v   |
|        | audit      | Trail, Log           |      |  v   |
| 維運層 | telemetry  | Collector, Rule      |      |  v   |
|        | protection | Backup, Restore      |      |  v   |
| 核心層 | compute    | VirtualMachine       |      |  v   |
|        | network    | VPC, IPPool          |      |  v   |
|        | storage    | BlockPool            |      |  v   |
| 實體層 | fleet      | Cluster              |  v   |      |
|        | metal      | Server               |  v   |      |
|        | link       | Tunnel               |  v   |      |
|        | bootstrap  | Config, Image        |  v   |      |

---

Level,Group,Kind,Core (Hub),Edge (Agent),設計意圖與備註
應用層,inference,"Service, Model",,v,AI 推論直接在 Edge GPU 跑，狀態存在 Edge。
,training,"FinetuneJob, Dataset",,v,訓練任務在 Edge 排程與執行。
,hub,"Release, Chart",,v,Helm Release 紀錄在 Edge，斷網也能 Rollback。
,database,"Redis, Postgres",,v,DB 實體與連線資訊都在 Edge。
,workflow,"Pipeline, Task",,v,類似 Tekton/Argo Workflow，在 Edge 跑。
治理層,identity,"Workspace, User",,v,Local RBAC。Edge 內部的權限控管。
,iam (新增),"Account, Tenant",v,,Global Auth。這是登入 Core 後台用的全域帳號。
,governance,"Policy, Compliance",,v,OPA/Kyverno 規則，由 Edge Enforce。
,finops,"Budget, CostReport",,v,Edge 計算自己的花費，Core 只負責拉報表。
,audit,"Trail, Log",,v,操作紀錄產生在 Edge。
維運層,telemetry,"Collector, Rule",,v,Prometheus/Otel Collector 跑在 Edge。
,protection,"Backup, Restore",,v,Velero 備份還原在 Edge 執行。
核心層,compute,VirtualMachine,,v,KubeVirt CRD。Core 透過 Proxy 操作。
,network,"VPC, IPPool",,v,CNI 設定。Core 透過 Proxy 操作。
,storage,BlockPool,,v,Ceph/CSI 設定。Core 透過 Proxy 操作。
實體層,fleet,Cluster,v,,艦隊名冊。紀錄 Edge 的連線資訊與狀態。
,metal,Server,v,,硬體庫存。管理未分配的裸機伺服器。
,link,TunnelServer,v,,Core 端的 Tunnel 監聽設定。
,,TunnelClient,,v,"Edge 端的連線設定 (Token, Endpoint)。"
,bootstrap,"Config, Image",v,,開機引導。Talos/Cloud-init 設定，只有 Core 知道。
