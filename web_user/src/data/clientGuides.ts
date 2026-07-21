/**
 * 客户端使用教程
 * - Windows / Android / macOS：统一 FlClash（本站 /downloads/）
 * - 图文：/downloads/docs-img/
 * - iOS：Shadowrocket
 */

export type GuideLink = { label: string; url: string; primary?: boolean }
export type GuideStep = { title: string; body: string; images?: string[] }
export type ClientGuide = {
  id: string
  platform: 'windows' | 'macos' | 'android' | 'ios'
  name: string
  summary: string
  badge?: string
  downloads: GuideLink[]
  steps: GuideStep[]
  tips?: string[]
}

export const platformTabs: { key: ClientGuide['platform']; label: string; icon: string }[] = [
  { key: 'windows', label: 'Windows', icon: '💻' },
  { key: 'macos', label: 'macOS', icon: '🍎' },
  { key: 'android', label: 'Android', icon: '📱' },
  { key: 'ios', label: 'iOS', icon: '📲' },
]

/** 本站静态资源前缀（与 Nginx /downloads/ 对应，拆用户端时可改成主站绝对域名） */
const DL = '/downloads'
const IMG = `${DL}/docs-img`

/** FlClash 版本：更新时改这里 + 服务器 /var/www/k2board-downloads/ 同步换包 */
const FL_VER = '0.8.94'
const flFile = (suffix: string) => `${DL}/FlClash-${FL_VER}-${suffix}`

const FL = {
  ui: `${IMG}/flclash-ui.jpeg`,
  smart1: `${IMG}/flclash-smart1.jpeg`,
  smart2: `${IMG}/flclash-smart2.jpeg`,
  add: `${IMG}/flclash-add.jpeg`,
  urlMenu: `${IMG}/flclash-url-menu.jpeg`,
  paste: `${IMG}/flclash-paste.jpeg`,
  ok: `${IMG}/flclash-ok.jpeg`,
  node: `${IMG}/flclash-node.jpeg`,
  proxy: `${IMG}/flclash-proxy.jpeg`,
  update: `${IMG}/flclash-update.jpeg`,
}

const SS = {
  store: `${IMG}/ios-store.webp`,
  plus: `${IMG}/ios-ss-1.png`,
  type: `${IMG}/ios-ss-2.png`,
  url: `${IMG}/ios-ss-3.png`,
  route: `${IMG}/ios-ss-4.png`,
  route2: `${IMG}/ios-ss-5.png`,
  sub: `${IMG}/ios-ss-6.png`,
}

const flclashSharedSteps: GuideStep[] = [
  {
    title: '复制本站订阅链接',
    body: '打开东京热云用户中心 → 点击「复制订阅链接」或订阅页「FlClash」按钮。默认已是 Clash 格式（flag=clash），可直接导入 FlClash。',
  },
  {
    title: '添加配置（URL 导入）',
    body:
      '1. 打开 FlClash，点左侧「配置」。\n' +
      '2. 点右下角「＋」。\n' +
      '3. 选择「URL / 通过 URL 获取配置文件」。\n' +
      '4. 粘贴刚才复制的订阅链接，确认导入。\n' +
      '5. 导入成功后，点选该配置以启用。',
    images: [FL.add, FL.urlMenu, FL.paste, FL.ok],
  },
  {
    title: '选择节点',
    body:
      '点左侧「代理」，选择地区节点。\n' +
      '可点右下角测延迟，数值越小通常越快（仅供参考）。',
    images: [FL.node],
  },
  {
    title: '开启代理',
    body:
      '1. 点左侧「仪表盘」。\n' +
      '2. 打开「系统代理」开关。\n' +
      '3. 出站模式建议选「规则」（国内直连、国外走代理，省流量）。\n' +
      '4. 点右下角开启连接。\n\n' +
      '出站模式说明：\n' +
      '· 规则：按配置分流（推荐）\n' +
      '· 全局：全部走代理\n' +
      '· 直连：不使用代理',
    images: [FL.proxy, FL.ui],
  },
  {
    title: '更新订阅（可选）',
    body: '在「配置」页点右上角更新图标，可刷新全部订阅，建议定期更新。',
    images: [FL.update],
  },
]

const flclashTips = [
  'Windows / Android / macOS 均为 FlClash，界面与操作基本一致，仅安装包不同。',
  '安装包与教程图均由本站 /downloads/ 提供，无需访问 GitHub。',
  '软件本身免费开源；订阅由本站提供。',
]

function flGuide(
  platform: 'windows' | 'macos' | 'android',
  nameExtra: string,
  downloads: GuideLink[],
  installStep: GuideStep,
): ClientGuide {
  return {
    id: `flclash-${platform}`,
    platform,
    name: `FlClash（${nameExtra}）`,
    summary: '基于 Clash Meta 的多端客户端：本站下载 → 导入订阅 → 选节点 → 开系统代理。',
    badge: '推荐',
    downloads,
    steps: [installStep, ...flclashSharedSteps],
    tips: flclashTips,
  }
}

export const clientGuides: ClientGuide[] = [
  flGuide(
    'windows',
    'Windows',
    [
      {
        label: 'Windows 安装包 (.exe)',
        url: flFile('windows-amd64-setup.exe'),
        primary: true,
      },
      {
        label: 'Windows 绿色版 (.zip)',
        url: flFile('windows-amd64.zip'),
      },
    ],
    {
      title: '下载并安装',
      body:
        '1. 点击上方绿色按钮下载 Windows 安装包。\n' +
        '2. 若提示「Windows 已保护你的电脑」→ 点「更多信息」→「仍要运行」。\n' +
        '3. 按默认选项完成安装。',
      images: [FL.smart1, FL.smart2],
    },
  ),

  flGuide(
    'macos',
    'macOS',
    [
      {
        label: 'Apple 芯片 (M 系列) .dmg',
        url: flFile('macos-arm64.dmg'),
        primary: true,
      },
      {
        label: 'Intel 芯片 .dmg',
        url: flFile('macos-amd64.dmg'),
        primary: true,
      },
    ],
    {
      title: '下载并安装',
      body:
        '1. 苹果菜单 →「关于本机」查看芯片：Apple 芯片选 arm64，Intel 选 amd64。\n' +
        '2. 下载对应 dmg，拖入「应用程序」。\n' +
        '3. 若无法打开：系统设置 → 隐私与安全性 → 仍要打开。',
      images: [FL.ui],
    },
  ),

  flGuide(
    'android',
    'Android',
    [
      {
        label: 'Android ARM64 安装包',
        url: flFile('android-arm64-v8a.apk'),
        primary: true,
      },
      {
        label: 'Android ARMv7（旧机）',
        url: flFile('android-armeabi-v7a.apk'),
      },
    ],
    {
      title: '下载并安装',
      body:
        '1. 下载 ARM64 APK（绝大多数手机）。\n' +
        '2. 允许「安装未知应用」后安装。\n' +
        '3. 首次启动按提示授予 VPN 等权限。',
      images: [FL.ui],
    },
  ),

  {
    id: 'ios-shadowrocket',
    platform: 'ios',
    name: 'Shadowrocket（小火箭）',
    summary:
      'iOS 推荐客户端。点击页面右下角 Crisp 客服气泡向客服索取临时美区 Apple ID，按图下载并导入订阅。',
    badge: 'iOS',
    downloads: [],
    steps: [
      {
        title: '向在线客服索取美区 Apple ID',
        body:
          '1. 点击本站页面右下角的 Crisp 客服图标（官方聊天气泡，颜色可能随主题变化）打开聊天窗口。\n' +
          '2. 直接向客服说明：需要下载 Shadowrocket 的临时美区 Apple ID。\n' +
          '3. 客服会提供账号与密码（仅用于登录 App Store 下载 App）。\n' +
          '4. 注意：请勿在「设置 → 登录 iPhone / iCloud」中登录该账号，只在 App Store 使用。\n' +
          '5. 若弹出 App 升级提示，请选择「不升级 / 稍后」。',
        images: [SS.store],
      },
      {
        title: '切换商店并下载（验证码找客服）',
        body:
          '1. 打开 iPhone「App Store」→ 右上角头像 → 滑到底部「退出登录」当前账号。\n' +
          '2. 用客服提供的美区 Apple ID 登录 App Store。\n' +
          '3. 登录或下载过程中，Apple 往往会向该 ID 绑定邮箱发送验证码；验证码会发到客服侧，不会到你自己的邮箱。\n' +
          '4. 看到需要输入验证码时：回到本站右下角 Crisp 客服窗口，向客服说明「已触发验证码，请提供验证码」。\n' +
          '5. 客服发来验证码后，填入 App Store 完成登录 / 下载。\n' +
          '6. 搜索并安装 Shadowrocket（小火箭）。软件费用由开发者收取，与本站无关。\n' +
          '7. 安装完成后，建议在 App Store 退出美区账号，改回自己的 Apple ID（App 已装好不受影响）。',
        images: [SS.store],
      },
      {
        title: '导入订阅',
        body:
          '1. 本站复制订阅链接（需已购买有效套餐）。\n' +
          '2. 打开 Shadowrocket → 右上角「＋」。\n' +
          '3. 类型改为 Subscribe。\n' +
          '4. URL 粘贴订阅链接 → 右上角保存。',
        images: [SS.plus, SS.type, SS.url],
      },
      {
        title: '启用代理',
        body:
          '1. 首页「全局路由」选「代理」。\n' +
          '2. 底栏「设置」→ 底部「订阅」→ 开启「打开时更新」。\n' +
          '3. 首页选中节点，打开顶部连接开关。',
        images: [SS.route, SS.route2, SS.sub],
      },
    ],
    tips: [
      '美区 Apple ID 与验证码一律通过本站右下角 Crisp 客服索取，不要在系统设置里登录该账号。',
      '验证码出现后务必立刻在客服窗口向客服索取，超时可能需重新触发登录。',
      '客服气泡外观/颜色由 Crisp 后台配置，可能与示意图不完全一致。',
      '建议定期打开 App 更新订阅。',
    ],
  },
]

export const quickStartSteps = [
  { t: '复制订阅', d: '点上方绿色按钮' },
  { t: '选系统', d: 'Win / Mac / 安卓 / iOS' },
  { t: '本站下载', d: '无需访问 GitHub' },
  { t: '导入开启', d: '按图操作即可' },
]
