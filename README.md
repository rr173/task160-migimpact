# task160-migimpact — 数据迁移影响范围分析服务

基于 REQ-20260822-022 生成。数据库平台工程师登记模式快照、服务访问声明与迁移脚本后，
服务构建对象依赖图，识别破坏性变更、受影响接口与安全执行次序，支持豁免审核与不可变计划冻结。

## 业务闭环

1. 登记 schema 快照（表/列/索引/视图/外键）与对象依赖边、服务访问声明。
2. 导入版本连续的迁移脚本（DDL 步骤，内容指纹幂等判重）。
3. 执行影响分析：破坏性变更分类（删列/删表/改类型/删被依赖索引等）+ 依赖传播 + 受影响接口。
4. 拓扑排序生成安全执行次序，检测循环依赖。
5. 对破坏性变更提交豁免；批准后冻结不可变迁移计划。
6. 重启后恢复排队中的分析任务，相同脚本指纹幂等。

## 标准命令

```bash
export GOTOOLCHAIN=local
go build ./...
go vet ./...
go test ./...
go run ./cmd/migimpact --smoke-test
```

## 启动服务

```bash
go run ./cmd/migimpact --addr :8080 --db migimpact.db
```

## API（前缀 /api）

- 快照：`POST /api/snapshots`、`GET /api/snapshots`、`GET /api/snapshots/{id}`、
  `POST /api/snapshots/{id}/objects`、`GET /api/snapshots/{id}/objects`、
  `GET /api/snapshots/{id}/dependencies`、`GET /api/snapshots/{id}/access`
- 脚本：`POST /api/scripts`、`GET /api/scripts`、`GET /api/scripts/{id}`、`GET /api/scripts/{id}/steps`
- 分析：`POST /api/scripts/{id}/analyze?snapshot_id=`、`GET /api/analyses`、`GET /api/analyses/{id}`、
  `GET /api/analyses/{id}/changes`、`POST /api/analyses/{id}/exemptions?change_id=`、`GET /api/analyses/{id}/exemptions`
- 计划：`POST /api/plans?analysis_id=`、`GET /api/plans`、`GET /api/plans/{id}`、`GET /api/plans/{id}/diff`
- 其他：`POST /api/example`、`GET /api/audit`、`GET /api/health`

## 示例（curl）

```bash
# 1. 创建快照
curl -s -X POST localhost:8080/api/snapshots -d '{"name":"prod-schema"}'

# 2. 填充对象/依赖/访问声明（用示例快速体验）
curl -s -X POST localhost:8080/api/example

# 3. 导入脚本
curl -s -X POST localhost:8080/api/scripts -d '{"name":"m1","version":1,"content":"1 DROP_TABLE [orders]\n"}'

# 4. 分析
curl -s "localhost:8080/api/scripts/1/analyze?snapshot_id=1"

# 5. 豁免并冻结
curl -s -X POST "localhost:8080/api/analyses/1/exemptions?change_id=1" -d '{"operator":"alice","reason":"已验证无下游"}'
curl -s -X POST "localhost:8080/api/plans?analysis_id=1"
```

## 持久化

SQLite（modernc.org/sqlite，CGO 无关）落盘 `migimpact.db`，保存快照、对象、依赖、访问声明、
脚本、步骤、分析、破坏性变更、豁免、计划与审计事件；服务重启自动恢复排队中的分析任务。
