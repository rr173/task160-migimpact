# BENZHI_README — 评测说明（task160-migimpact）

## 项目

数据迁移影响范围分析服务：登记数据库模式快照与迁移脚本，构建对象依赖图，
识别破坏性变更与受影响接口，生成安全执行次序，支持豁免审核与不可变计划冻结。
纯后端服务，SQLite（modernc.org/sqlite）持久化，具备重启恢复与幂等导入。

## 构建与自检命令（评测机执行，全部需真实成功）

```bash
export GOTOOLCHAIN=local
go build ./...
go vet ./...
go test ./...
go run ./cmd/migimpact --smoke-test
```

- `--smoke-test` 契约：创建示例快照 → 填充对象/依赖/访问声明 → 导入含破坏性变更的脚本 →
  执行影响分析（断言识别到破坏性变更）→ 提交豁免 → 冻结不可变计划 → 关闭并重开数据库验证持久化 → 退出码 0。
- 服务入口：`migimpact --addr :8080 --db migimpact.db`，路由前缀 `/api`。

## Docker 双架构验证

```bash
bash build_benzhi_docker.sh task160-migimpact linux/amd64
docker run --rm task160-migimpact --smoke-test   # 必须退出码 0

bash build_benzhi_docker.sh task160-migimpact linux/arm64
docker run --rm task160-migimpact --smoke-test   # 必须退出码 0
```

- 单阶段构建：`golang:1.26.3-bookworm` + `CGO_ENABLED=0 go build ./cmd/migimpact`，产物置于 alpine 运行镜像。
- 镜像内执行 `--smoke-test` 后自行退出，不依赖任何外部服务。

## 环境版本

- Go：1.26.3（GOTOOLCHAIN=local）
- SQLite：3.46.1（经 modernc.org/sqlite v1.52.0，CGO_ENABLED=0 可构建）
- component-versions.json 与 go.mod / Dockerfile 一致。

## API 摘要

快照（create/list/get/objects/deps/access）、脚本（import/list/get/steps）、
分析（analyze/list/get/changes/exemptions）、计划（freeze/list/get/diff）、
示例（POST /api/example）、审计（GET /api/audit）、健康（GET /api/health）。

## 错误边界

拒绝悬空依赖、脚本版本不连续、循环依赖执行次序、未完成分析冻结、重复脚本指纹（幂等返回既有）、
已冻结计划改写（不可修改）。已批准计划绑定原始快照哈希与步骤集合。
