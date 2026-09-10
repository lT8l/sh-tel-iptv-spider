# sh-tel-iptv-spider

上海电信 IPTV 静态 XMLTV / M3U8 生成器。

程序一次运行完成 IPTV 认证、频道与 EPG 抓取，然后写出静态 `channels.xml` 和 `channels.m3u8` 并退出。不启动 Web 服务，也不依赖数据库、缓存、对象存储或内置定时任务。

## 环境要求

- 上海电信 IPTV 机顶盒 UID / SN / MAC。
- Linux 设备存在可访问上海电信 IPTV 专网的独立网卡或接口。
- Go 1.23 或更高版本。

## 配置

复制示例配置后填写机顶盒信息：

```bash
cp config.example.yaml config.yaml
```

主要配置：

- `stb`：机顶盒认证信息以及 IPTV 专网出口网卡。`stb.interface` 填接口名（如 `eth1`）；程序读取该接口当前 IPv4 用于认证，并通过 Linux `SO_BINDTODEVICE` 将 HTTP 连接和 DNS 查询都强制绑定到该接口，不会回退到系统默认出口。
- `epg`：XMLTV 元数据、抓取时间窗口和时区。
- `categories`：`getChannelList` 使用的 `cateID/type` 组合。
- `output`：输出文件名、直播地址格式、FCC、catchup 和购物频道过滤。
- `request`：HTTP 超时和连续请求间隔。

严格网卡绑定仅支持 Linux；如果运行环境不允许 `SO_BINDTODEVICE`，网络请求会直接失败，而不是改走其他网卡。

`output.stream_mode` 仅支持：

- `rtp`：生成 `rtp://host:port`；`output.fcc: true` 时可附加频道返回的 FCC 地址。
- `udp`：生成 `udp://host:port`。
- `raw`：保留运营商返回的原始直播地址。

`output.catchup_days > 0` 且频道存在有效 `TimeShiftURL` 时，会在 M3U8 中写入标准 catchup 属性。`catchup_template` 默认使用：

```text
playseek=${(b)yyyyMMddHHmmss}-${(e)yyyyMMddHHmmss}
```

## 运行

```bash
go run . -config config.yaml
```

示例配置默认在当前目录生成：

```text
channels.m3u8
channels.xml
```

## 频道处理

- `CommName` 是去除 HD / 4K 等清晰度标记后的标准节目频道名。
- 同一逻辑频道的 SD / HD / 4K 共享同一个 `CommName` 和 `MixNo`；栏目抓取不会再按 `MixNo` 提前合并掉不同清晰度。
- 同一 `CommName` 最多保留一个 4K 和一个 HD 播放源；只要二者任一存在，就不再保留对应 SD 播放源。
- 不同清晰度共享 M3U8 `tvg-id`、XMLTV `channel id` 和同一份 programme；`DisplayName` 用于各播放源以及 XMLTV 的显示名称。
- `IsCharge` 仅保留运营商原始字段，不参与频道筛选或优先级。
- `MixNo` 仅用于内部频道关联、EPG 请求去重和频道排序，不写入 M3U8/XMLTV 的频道 ID。
- EPG 按 `MixNo` 去重，每个逻辑频道只请求一次，不做清晰度或 MixNo 优先级选择。
- M3U8 的受管理频道按 `MixNo` 数值升序生成；已有但未匹配的手工频道保持原样。
- 默认过滤购物频道，可通过 `output.include_shopping: true` 开启。

## M3U8 增量更新

如果目标 M3U8 已存在，程序不会整表覆盖：

- 以 `DisplayName` 匹配已有频道。
- 程序管理的 `tvg-id`、`tvg-name`、`group-title`、`catchup*`、显示名称和直播 URL 以本次生成结果为准；手工维护的 `tvg-logo` 等未知/自定义属性会保留。
- 新发现频道会增加。
- 已有但本次未发现的频道不会删除。
- 本次匹配或新增的受管理频道保持本次按 `MixNo` 生成的顺序；未匹配的既有频道原样保留。

XMLTV 每次重新生成，以保证节目单与当前抓取结果一致。

## 定时运行

需要周期更新时，请使用系统 `cron`、systemd timer 或其他外部调度器执行本程序。

## 免责声明

本项目仅供学习和研究使用。请遵守当地法律法规及运营商服务条款，并自行承担使用风险。
