// ignore: unused_import
import 'package:intl/intl.dart' as intl;
import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for Chinese (`zh`).
class AppLocalizationsZh extends AppLocalizations {
  AppLocalizationsZh([String locale = 'zh']) : super(locale);

  @override
  String get appTitle => 'Lanos';

  @override
  String get home => '首页';

  @override
  String get records => '记录';

  @override
  String get settings => '设置';

  @override
  String get transferRecords => '传输记录';

  @override
  String get shareRecords => '分享记录';

  @override
  String get noTransferRecords => '暂无传输记录';

  @override
  String get noTransferRecordsHint => '发送或接收文件后，将在此处显示记录';

  @override
  String get noShareRecords => '暂无分享记录';

  @override
  String get noShareRecordsHint => '通过网页链接分享文件后，将在此处显示记录';

  @override
  String get search => '搜索';

  @override
  String get sort => '排序';

  @override
  String get sortByTime => '按时间';

  @override
  String get sortBySize => '按大小';

  @override
  String get sortByName => '按名称';

  @override
  String get sortByStatus => '按状态';

  @override
  String get ascending => '升序';

  @override
  String get descending => '降序';

  @override
  String get selectAll => '全选';

  @override
  String get delete => '删除';

  @override
  String get deleteSelected => '删除选中';

  @override
  String deleteConfirm(Object count) {
    return '确定删除 $count 条记录？';
  }

  @override
  String get cancel => '取消';

  @override
  String get confirm => '确认';

  @override
  String get exportCSV => '导出 CSV';

  @override
  String get statusCompleted => '已完成';

  @override
  String get statusFailed => '失败';

  @override
  String get statusCancelled => '已取消';

  @override
  String get statusActive => '进行中';

  @override
  String get statusExpired => '已过期';

  @override
  String get statusStopped => '已停止';

  @override
  String get statusPending => '等待中';

  @override
  String get directionSend => '发出';

  @override
  String get directionReceive => '接收';

  @override
  String sizePrefix(Object size) {
    return '大小：$size';
  }

  @override
  String timePrefix(Object time) {
    return '时间：$time';
  }

  @override
  String peerPrefix(Object name) {
    return '对方：$name';
  }

  @override
  String get deviceName => '设备名称';

  @override
  String get downloadPath => '下载路径';

  @override
  String get autoReceive => '自动接收';

  @override
  String get autoReceiveAsk => '询问';

  @override
  String get autoReceiveTrusted => '仅信任设备';

  @override
  String get autoReceiveAll => '所有设备';

  @override
  String get conflictPolicy => '文件冲突';

  @override
  String get conflictPolicySkip => '跳过';

  @override
  String get conflictPolicyOverwrite => '覆盖';

  @override
  String get conflictPolicyKeepBoth => '保留两者';

  @override
  String get stealthMode => '隐身模式';

  @override
  String get stealthModeHint => '对其他设备不可见';

  @override
  String get ipv6Preferred => '优先 IPv6';

  @override
  String get ipv6PreferredHint => '可用时优先使用 IPv6';

  @override
  String get notifyReceive => '接收通知';

  @override
  String get notifySent => '发送通知';

  @override
  String get notifyShare => '分享通知';

  @override
  String get recordRetention => '记录保留';

  @override
  String get recordRetentionUnlimited => '无限';

  @override
  String recordsCount(Object count) {
    return '$count 条记录';
  }

  @override
  String get language => '语言';

  @override
  String get languageSystem => '跟随系统';

  @override
  String get languageZh => '中文';

  @override
  String get languageEn => 'English';

  @override
  String get about => '关于';

  @override
  String get version => '版本';

  @override
  String get onboardingWelcome => '欢迎使用 Lanos';

  @override
  String get onboardingWelcomeDesc => '在局域网中安全地分享文件。无需互联网，无需文件大小限制。';

  @override
  String get onboardingSend => '发送文件';

  @override
  String get onboardingSendDesc => '选择文件，选择目标设备，端到端加密传输。';

  @override
  String get onboardingReceive => '接收文件';

  @override
  String get onboardingReceiveDesc => '接收附近设备的传输，或通过网页链接轻松下载。';

  @override
  String get onboardingGetStarted => '开始使用';

  @override
  String get onboardingSkip => '跳过';

  @override
  String get onboardingNext => '下一步';

  @override
  String get unitBytes => 'B';

  @override
  String get unitKB => 'KB';

  @override
  String get unitMB => 'MB';

  @override
  String get unitGB => 'GB';

  @override
  String get bootingDaemon => '正在启动 Lanos 守护进程...';

  @override
  String get bootFailed => '启动失败';

  @override
  String get retryBoot => '重试启动';

  @override
  String get refreshDevices => '刷新设备列表';

  @override
  String get nearbyDevices => '附近设备';

  @override
  String get sendFile => '发送文件';

  @override
  String get searchingDevices => '正在搜索附近设备...';

  @override
  String get searchingDevicesHint => '确保其他设备已安装 Lanos 并连接同一 Wi-Fi';

  @override
  String get selectDevice => '选择设备';

  @override
  String get sendingStarted => '开始发送';

  @override
  String sendFailed(Object error) {
    return '发送失败：$error';
  }

  @override
  String get reconnectingGcd => '正在重新连接 gcd…';

  @override
  String get transfers => '传输';

  @override
  String get outgoing => '上行';

  @override
  String get incoming => '下行';

  @override
  String get noTransferProgress => '暂无传输记录';

  @override
  String get noTransferProgressHint => '发送或接收文件后，将在此处显示传输进度和记录';

  @override
  String get receive => '接收';

  @override
  String get noReceiveRequests => '暂无接收请求';

  @override
  String get noReceiveRequestsHint => '当其他设备向你发送文件时，将在此处显示';

  @override
  String get deleteConfirmTitle => '删除确认';

  @override
  String get csvObtained => 'CSV 已获取，可复制到文件';

  @override
  String get close => '关闭';

  @override
  String exportFailed(Object error) {
    return '导出失败: $error';
  }

  @override
  String get exitSelect => '退出选择';

  @override
  String get multiSelect => '多选';

  @override
  String get exportTransfersCSV => '导出传输记录 CSV';

  @override
  String get exportSharesCSV => '导出分享记录 CSV';

  @override
  String get searchHint => '按文件名搜索...';

  @override
  String searchQueryHint(Object query) {
    return '搜索: $query (需后端 API 完善)';
  }

  @override
  String downloadsCount(Object count) {
    return '$count 次下载';
  }

  @override
  String minutesAgo(Object count) {
    return '$count 分钟前';
  }

  @override
  String hoursAgo(Object count) {
    return '$count 小时前';
  }

  @override
  String daysAgo(Object count) {
    return '$count 天前';
  }

  @override
  String monthDay(Object day, Object month) {
    return '$month/$day';
  }

  @override
  String get statusWaiting => '等待';

  @override
  String get statusTransferring => '传输中';

  @override
  String loadSettingsFailed(Object error) {
    return '加载设置失败: $error';
  }

  @override
  String get settingsSaved => '设置已保存';

  @override
  String get saveSettings => '保存设置';

  @override
  String saveFailed(Object error) {
    return '保存失败: $error';
  }

  @override
  String get sectionDevice => '设备';

  @override
  String get sectionTransfer => '传输';

  @override
  String get sectionNetwork => '网络';

  @override
  String get sectionNotify => '通知';

  @override
  String get sectionStorage => '存储';

  @override
  String get sectionAbout => '关于';

  @override
  String recordsN(Object count) {
    return '$count 条';
  }

  @override
  String versionValue(Object version) {
    return '版本 $version';
  }

  @override
  String get cancelTooltip => '取消';

  @override
  String get sasConfirmTitle => '确认配对';

  @override
  String sasVerifyPrompt(Object name) {
    return '请与 \"$name\" 核对以下数字是否一致，';
  }

  @override
  String get sasVerifyHint => '一致后点击\"确认\"以建立加密连接。';

  @override
  String sasAutoCancel(Object seconds) {
    return '$seconds 秒后自动取消';
  }

  @override
  String get thisDevice => '本机';
}
