# 声轨通无障碍字幕质检台

这是一个面向广播节目制作团队的字幕母版质检与交付工作台。系统以单一 Go 服务提供响应式浏览器页面和同源 JSON API，支持创建字幕包、录入带时间码的字幕条目、自动规则质检、人工审校、退回修订、冻结 WebVTT 母版以及签发不可变交付凭据。数据保存在本地事件日志和快照中，服务重启后会校验并恢复投影。

## 构建

```text
go build ./cmd/subtitleqc
```

## 运行

```text
go run ./cmd/subtitleqc -addr=127.0.0.1:19137
```

也可以设置数字型 `PORT`，服务会绑定 `127.0.0.1:<PORT>`。未指定时默认监听 `127.0.0.1:19137`。数据目录默认为工作区下的 `data`，可使用 `-data-dir` 指定。

浏览器打开 `http://127.0.0.1:19137/`，页面通过 `/api/v1/subtitle-packages` 及子资源完成主流程。

工作台支持按状态、节目标题、期号和质检规则筛选字幕包，并显示完整状态统计。草稿详情可粘贴 `开始毫秒|结束毫秒|说话人|字幕文本|声音提示` 格式的逐行文本进行无写入预览，再确认导入。质检详情提供分级汇总、字幕条目定位和批量处置；审校员还可按 cue 或包内时间范围登记人工问题，人工与自动发现进入同一处置及复审门禁。

需要修订的字幕包可选择多条 cue 和本批次声明解决的发现，先执行无写入影响预检，再提交带字段差异和发现关联证据的修订批次。未关联发现会继续保留，修订后必须重新质检才能再次提交复审。复审通过后先生成确定性的 WebVTT 冻结预检，页面展示摘要、条数、完整内容和 SHA-256；确认请求同时匹配预检版本和校验和后才能冻结。

工作台的“质检统计”视图按更新时间、语言和交付标准汇总字幕包、交付、首次通过、退回和返修数据，并按 `ruleCode`、`severity`、`source`、`disposition` 展示稳定排序的发现分布。点击规则行可回到对应字幕包列表，统计查询全程只读。

新增业务入口包括 `POST /api/v1/subtitle-packages/{id}/manual-findings`、`POST /api/v1/subtitle-packages/{id}/revisions/preview`、`GET /api/v1/subtitle-packages/{id}/freeze-preview` 和 `GET /api/v1/statistics/quality`。冻结确认仍使用 `POST /api/v1/subtitle-packages/{id}/freeze`，请求必须携带预检得到的 `expectedVersion`、`expectedChecksum` 和非空 `idempotencyKey`。

其他只读接口包括 `/api/v1/subtitle-packages/{id}/findings`、`/api/v1/subtitle-packages/{id}/master`、`/api/v1/subtitle-packages/{id}/credential`、`/api/v1/credentials` 和 `/api/v1/audit/{id}`。审计接口支持 `type`、`actor`、`from`、`to` 和 `limit` 查询参数；统计接口支持 `from`、`to`、`language` 和 `deliveryStandard`，日期可使用 `YYYY-MM-DD` 或 RFC 3339。

## 测试与自检

```text
go test ./...
go run ./cmd/subtitleqc -selfcheck -addr=127.0.0.1:19137
```

自检会启动真实 HTTP 服务，在临时目录中完成建包、字幕准备、质检、复审、冻结和交付，然后主动关闭服务。
