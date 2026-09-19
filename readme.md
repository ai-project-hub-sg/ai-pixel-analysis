1.机密信息在.env中
2.在.env中user_开头为用户信息。成组出现。以最后的_number区分。
3.在.env中_port结尾为相关接口地址，与host结合为完整的网址。
4.在.env中db_secret表示数据库存储机密信息时，使用此密钥来加密，需要加盐，拼接规则为机密信息:盐值然后整体加密。使用对称加密，密钥要能被恢复。
4.本项目使用go语言实现，当前编译为windwos版本运行，后期编译为liunx运行的服务器版本。
5.本项目使用sqlite作为数据库
6.不要将.env中的机密信息回显和硬编码到任何位置，包括注释。

以下为项目进度
1.实现通过登录接口进行登录，获取authorization和cookie供后续业务调用，状态已完成。可以通过chrome先抓包一下登录相关的代码和数据。人在登录的时候发现login接口post的payload除了账户密码外还有个login_agreement_revision，这个怎么来的，需要弄清楚。不然go登录会有问题。获取到authorization和cookie存储在sqlite中。供后续使用

2.获取数据，状态已完成。以下为具体实现情况：
2.1已确认"我的账号->查看统计"页涉及接口：使用明细 GET /api/v1/usage（分页）、使用汇总 GET /api/v1/usage/stats、额度窗口 GET /api/v1/usage/dashboard/stats、趋势 GET /api/v1/usage/dashboard/trend、模型拆分 GET /api/v1/usage/dashboard/models。请求参数与返回结构见 docs/api.md。
2.2已确认"使用记录->余额流水"页接口：分页流水 GET /api/v1/usage/balance-ledger、流水汇总 GET /api/v1/usage/balance-ledger/stats。
2.3已实现按日期区间(-start/-end)、精确时间区间(-start-time/-end-time)、快捷周期(-period today|yesterday|last7days|last30days)抓取数据。
2.4原始响应逐页落盘到 data/raw/<类别>/<账号>__<范围>_p<页码>.json。
2.5清洗字段取舍见 docs/api.md 字段表；只保留计费与分析必需字段，丢弃每条重复的 user/api_key/group 嵌套对象。
2.6清洗后 upsert 进 sqlite 的 usage_logs 与 balance_ledger 表（含 metadata 明细）。
2.7 export 命令从数据库生成 md 报告（汇总/按模型/按天/流水分类/最近50条含使用者、key、账户、请求ID）。

3.数据分析，状态已完成
已实现 web 前端界面（新增 web 子命令，只读模式打开 sqlite，纯 Go net/http + embed 静态资源 + 原生 JS/SVG 图表，零外部依赖，单 exe 可跑）。

3.1 使用方式：双击 `start-web.bat`（推荐，自动进入目录并启动），或命令行运行 `ai-pixel-analysis web [-addr :8080]`，浏览器访问 http://localhost:8080。不加载 .env（不接触加密凭据），数据库以 mode=ro 打开，驱动层禁止写入，落实"严禁增删改"。

3.2 四个分页（顶部 tab 切换，右上角可按账号筛选）：
- 总览：账号数、7D 总请求、分账收入合计、当前/上一小时分账收入、小时环比(vs上一小时)、小时同比(vs昨天同小时)、今日收入、今日时均收入；每账号一行含 7D请求/占比、账户计费(total_cost)、用户扣费(actual_cost)、分账收入、各同环比、最新余额。
- 分账收入：逐小时/按天收入趋势图（稀疏补0，横轴连续）、收入 Top 使用者(consumer_user_id)、Top 上游账户(account_id)。
- 用量分析：总请求/tokens/计费/实扣/均耗时卡片、按模型明细表、request_type/billing_mode/stream 分布、按天用量趋势+明细表(含毛利=分账收入-用户扣费)、单日24小时分布。
- 流水明细：按方向(credit/debit)/原因筛选、分页表格，含金额/余额/使用者/Key/账户/请求ID。

3.3 分析口径：时间字段为 RFC3339 含时区，sqlite substr(created_at,1,13) 取本地小时桶。环比=当前周期/紧邻上周期−1；同比=当前小时/昨天同一小时−1；分母为0显示"—"避免误导。今日时均=今日累计分账收入÷已过去小时数。

3.4 推荐分析维度及理由：分账收入趋势(时/天)——核心业务指标识别高峰与拐点；收入Top使用者/上游账户——定位头部贡献者指导运营；按模型用量——模型成本差异大需拆分；毛利(收入-扣费)——反映分账后净留存；请求类型/耗时——评估负载特征。

3.5 不分析的字段及原因：request_id/ref_id(逐条唯一，聚合无意义仅追溯用)；balance_after逐点(时点快照，按天采样已足够)；user_agent(与计费收入无直接关联)；first_token_ms(非流式为空样本缺失多聚合失真)；inbound_endpoint/group_id(取值单一区分度不足)。

使用方式见 docs/usage.md。重要:只允许请求数据，严禁对账号进行增删改操作。
