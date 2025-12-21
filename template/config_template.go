package template

const ConfigTemplate = `
version: 1
  ## 基本配置
settings:
  ws_address: ["ws://<YOUR_WS_ADDRESS>:<YOUR_WS_PORT>"] # WebSocket服务的地址 支持多个["","",""]
  app_id: 12345                             # 你的应用ID
  token: "<YOUR_APP_TOKEN>"                          # 你的应用令牌
  client_secret: "<YOUR_CLIENT_SECRET>"              # 你的客户端密钥

  ## onebot适配器配置
  hash_id : true                   #使用hash来进行idmaps转换,可以让user_id不是123开始的递增值
  use_requestid : true               #是否采用 requestID 避免远端插件无法获知发起者，false则使用echo
  auto_reply_message : "你输入的指令好像不对哦,请@机器人来获取可用指令"      #自动回复的消息内容，当auto_reply为true时生效
  command_whitelist: ["help", "pr", "re", "info", "bp", "bind"]  #指令白名单，只有这些指令会上报到ws服务器，留空则所有消息都上报
  auto_reply : true                #是否对所有收到的消息自动回复（不会上报给onebot应用）
  config_auto_reload : false         #配置文件热加载，检测到config.yml变动时自动重启程序应用新配置

  ## 公域机器人指令处理选项
  remove_prefix : true  #是否忽略公域机器人指令前第一个/
  remove_at : true      #是否忽略公域机器人指令前第一个[CQ:aq,qq=机器人] 场景(公域机器人,但插件未适配at开头)

  #事件订阅
  text_intent:                                       # 请根据公域 私域来选择intent,错误的intent将连接失败
    # - "ATMessageEventHandler"                        # 频道at信息
    - "DirectMessageHandler"                         # 私域频道私信(dms)
    - "ReadyHandler"                                # 连接成功
    - "ErrorNotifyHandler"                         # 连接关闭
    # - "GuildEventHandler"                          # 频道事件
    # - "MemberEventHandler"                         # 频道成员新增
    # - "ChannelEventHandler"                        # 频道事件
    # - "CreateMessageHandler"                       # 频道不at信息 私域机器人需要开启 公域机器人开启会连接失败
    # - "InteractionHandler"                         # 添加频道互动回应 卡片按钮data回调事件
    - "GroupATMessageEventHandler"                 # 群at信息 仅频道机器人时候需要注释
    - "C2CMessageEventHandler"                     # 群私聊 仅频道机器人时候需要注释
    # - "ThreadEventHandler"                         # 频道发帖事件 仅频道私域机器人可用

  global_channel_to_group: true                      # 是否将频道转换成群 默认true
  global_private_to_channel: false                   # 是否将私聊转换成频道 如果是群场景 会将私聊转为群(方便提审\测试)
  array: false

  ## idmaps和图床服务配置
  server_dir: "<YOUR_SERVER_DIR>" # 提供图片上传服务的服务器(图床)需要带端口号. 如果需要发base64图,需为公网ip,且开放对应端口
  port: "15630"                                # idmaps和图床对外开放的端口号
  lotus: false                                       # lotus特性默认为false,当为true时,将会连接到另一个lotus为false的gensokyo。
                                                     # 使用它提供的图床和idmaps服务(场景:同一个机器人在不同服务器运行,或内网需要发送base64图)。
                                                     # 如果需要发送base64图片,需要设置正确的公网server_dir和开放对应的port


  ## 正向WebSocket连接配置
  ws_token: ["","",""]      #连接wss地址时服务器所需的token,如果是ws,可留空,按顺序一一对应
  master_id : ["1","2"]     #群场景尚未开放获取管理员和列表能力,手动从日志中获取需要设置为管理,的user_id并填入(适用插件有权限判断场景)
  enable_ws_server: true    #是否启用正向ws服务器 监听server_dir:port/ws
  ws_server_token : "12345" #正向ws的token 不启动正向ws可忽略
  identify_file: true  #自动生成域名校验文件,在q.qq.com配置信息URL,在server_dir填入自己已备案域名,正确解析到机器人所在服务器ip地址,机器人即可发送链接
  crt: "" #证书路径 从你的域名服务商或云服务商申请签发SSL证书(qq要求SSL)
  key: "" #密钥路径 Apache（crt文件、key文件）示例: "C:\\123.key" \需要双写成\\
  developer_log : false    #开启开发者日志 默认关闭
  log_level : "info"      #日志级别: error, warn, info, debug (默认info)  image_sizelimit : 0   #代表kb 腾讯api要求图片1500ms完成传输 如果图片发不出 请提升上行或设置此值 默认为0 不压缩


  backup_port : "5200"   #当totus为ture时,port值不再是本地webui的端口,使用lotus_Port来访问webui
  develop_access_token_dir : ""     #开发者测试环境access_token自定义获取地址 默认留空 请留空忽略
  develop_bot_id : "1234"           #开发者环境需自行获取botid 填入 用户请不要设置这两行...开发者调试用
  sandbox_mode : false              #默认false 如果你只希望沙箱频道使用,请改为true
  title : "Gensokyo © 2023 - Hoshinonyaruko"              #程序的标题 如果多个机器人 可根据标题区分
`
const Logo = `
'
'    ,hakurei,                                                      ka
'   ho"'     iki                                                    gu
'  ra'                                                              ya
'  is              ,kochiya,    ,sanae,    ,Remilia,   ,Scarlet,    fl   and  yu        ya   ,Flandre,
'  an      Reimu  'Dai   sei  yas     aka  Rei    sen  Ten     shi  re  sca    yu      ku'  ta"     "ko
'  Jun        ko  Kirisame""  ka       na    Izayoi,   sa       ig  Koishi       ko   mo'   ta       ga
'   you.     rei  sui   riya  ko       hi  Ina    baI  'ran   you   ka  rlet      komei'    "ra,   ,sa"
'     "Marisa"      Suwako    ji       na   "Sakuya"'   "Cirno"'    bu     sen     yu''        Satori
'                                                                                ka'
'                                                                               ri'
`
