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

3.数据分析，未开始
做一个web前端界面，方便对数据进行分析。要求首页展示账号，7D使用量百分比，账户计费，用户扣费，号主分账收入。然后当前小时的分账收入，上一小时的分账收入。环比，同比。今天的小时均收入。你来补充哪些数据维度需要进行分析，需要进行哪些分析，汇总，均值，比例，同环比。等等。需要推荐分析的理由，如果有些字段不需要分析，也要给出不分析的原因。web页面可以是多分页。不要所有数据堆在一起开。要详略得当。

使用方式见 docs/usage.md。重要:只允许请求数据，严禁对账号进行增删改操作。
