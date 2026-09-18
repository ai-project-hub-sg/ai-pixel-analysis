1.机密信息在.env中
2.在.env中user_开头为用户信息。成组出现。以最后的_number区分。
3.在.env中_port结尾为相关接口地址，与host结合为完整的网址。
4.在.env中db_secret表示数据库存储机密信息时，使用此密钥来加密，需要加盐，拼接规则为机密信息:盐值然后整体加密。使用对称加密，密钥要能被恢复。
4.本项目使用go语言实现，当前编译为windwos版本运行，后期编译为liunx运行的服务器版本。
5.本项目使用sqlite作为数据库
6.不要将.env中的机密信息回显和硬编码到任何位置，包括注释。

以下为项目进度
1.实现通过登录接口进行登录，获取authorization和cookie供后续业务调用，状态已完成。可以通过chrome先抓包一下登录相关的代码和数据。人在登录的时候发现login接口post的payload除了账户密码外还有个login_agreement_revision，这个怎么来的，需要弄清楚。不然go登录会有问题。获取到authorization和cookie存储在sqlite中。供后续使用
