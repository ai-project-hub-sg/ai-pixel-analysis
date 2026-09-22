1.机密信息在.env中
2.在.env中user_开头为用户信息。成组出现。以最后的_number区分。
3.在.env中db_secret表示数据库存储机密信息时，使用此密钥来加密，需要加盐，拼接规则为机密信息:盐值然后整体加密。使用对称加密，密钥要能被恢复。
4.本项目使用go语言实现，当前编译为windwos版本运行，后期编译为liunx运行的服务器版本。
5.本项目使用sqlite作为数据库
6.不要将.env中的机密信息回显和硬编码到任何位置，包括注释。
7.接口地址统一在config.toml的[server]中配置，不要硬编码在代码里。

以下为项目进度
一、原始数据抽取，状态已完成（version2 重构）。
- 登录：GET {host}/login 提取 login_agreement_revision，POST /api/v1/auth/login 获取 authorization，加盐加密存入 sqlite。
- 账号列表：GET /api/v1/accounts 分页拉取，解析 name/account_id/account_level/concurrency/status 入库。
- 账号用量：GET /api/v1/accounts/{id}/usage 解析 utilization/cost/standard_cost/user_cost，计算 estimated_total_quota=cost/utilization（utilization=0 记为 -999）。
- 账号状态：GET /api/v1/accounts/{id}/stats 原始 JSON 落盘 + models 数据入库。
- 余额流水：GET /api/v1/usage/balance-ledger 分页拉取，从 metadata 清洗 consumer_user_id/api_key_id/account_id/request_id 入库。
- 原始响应统一落盘 data/raw/，清洗后入 sqlite；每次运行生成报告到 data/reports/。
- 接口文档见 docs/api.md。
