// ignore: unused_import
import 'package:intl/intl.dart' as intl;
import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for English (`en`).
class AppLocalizationsEn extends AppLocalizations {
  AppLocalizationsEn([String locale = 'en']) : super(locale);

  @override
  String get appTitle => 'Lanos';

  @override
  String get home => 'Home';

  @override
  String get records => 'Records';

  @override
  String get settings => 'Settings';

  @override
  String get transferRecords => 'Transfers';

  @override
  String get shareRecords => 'Shares';

  @override
  String get noTransferRecords => 'No transfer records';

  @override
  String get noTransferRecordsHint => 'Send or receive files to see them here';

  @override
  String get noShareRecords => 'No share records';

  @override
  String get noShareRecordsHint => 'Share files via web link to see them here';

  @override
  String get search => 'Search';

  @override
  String get sort => 'Sort';

  @override
  String get sortByTime => 'Time';

  @override
  String get sortBySize => 'Size';

  @override
  String get sortByName => 'Name';

  @override
  String get sortByStatus => 'Status';

  @override
  String get ascending => 'Ascending';

  @override
  String get descending => 'Descending';

  @override
  String get selectAll => 'Select All';

  @override
  String get delete => 'Delete';

  @override
  String get deleteSelected => 'Delete selected';

  @override
  String deleteConfirm(Object count) {
    return 'Are you sure you want to delete $count record(s)?';
  }

  @override
  String get cancel => 'Cancel';

  @override
  String get confirm => 'Confirm';

  @override
  String get exportCSV => 'Export CSV';

  @override
  String get statusCompleted => 'Completed';

  @override
  String get statusFailed => 'Failed';

  @override
  String get statusCancelled => 'Cancelled';

  @override
  String get statusActive => 'Active';

  @override
  String get statusExpired => 'Expired';

  @override
  String get statusStopped => 'Stopped';

  @override
  String get statusPending => 'Pending';

  @override
  String get directionSend => 'Sent';

  @override
  String get directionReceive => 'Received';

  @override
  String sizePrefix(Object size) {
    return 'Size: $size';
  }

  @override
  String timePrefix(Object time) {
    return 'Time: $time';
  }

  @override
  String peerPrefix(Object name) {
    return 'Peer: $name';
  }

  @override
  String get deviceName => 'Device Name';

  @override
  String get downloadPath => 'Download Path';

  @override
  String get autoReceive => 'Auto Receive';

  @override
  String get autoReceiveAsk => 'Ask';

  @override
  String get autoReceiveTrusted => 'Trusted devices only';

  @override
  String get autoReceiveAll => 'All devices';

  @override
  String get conflictPolicy => 'File Conflict';

  @override
  String get conflictPolicySkip => 'Skip';

  @override
  String get conflictPolicyOverwrite => 'Overwrite';

  @override
  String get conflictPolicyKeepBoth => 'Keep Both';

  @override
  String get stealthMode => 'Stealth Mode';

  @override
  String get stealthModeHint => 'Hide from other devices';

  @override
  String get ipv6Preferred => 'Prefer IPv6';

  @override
  String get ipv6PreferredHint => 'Use IPv6 when available';

  @override
  String get notifyReceive => 'Notify on Receive';

  @override
  String get notifySent => 'Notify on Sent';

  @override
  String get notifyShare => 'Notify on Share';

  @override
  String get recordRetention => 'Record Retention';

  @override
  String get recordRetentionUnlimited => 'Unlimited';

  @override
  String recordsCount(Object count) {
    return '$count records';
  }

  @override
  String get language => 'Language';

  @override
  String get languageSystem => 'System default';

  @override
  String get languageZh => '中文';

  @override
  String get languageEn => 'English';

  @override
  String get about => 'About';

  @override
  String get version => 'Version';

  @override
  String get onboardingWelcome => 'Welcome to Lanos';

  @override
  String get onboardingWelcomeDesc =>
      'Share files securely over local network. No internet required, no size limits.';

  @override
  String get onboardingSend => 'Send Files';

  @override
  String get onboardingSendDesc =>
      'Select files, choose a device, and transfer with end-to-end encryption.';

  @override
  String get onboardingReceive => 'Receive Files';

  @override
  String get onboardingReceiveDesc =>
      'Accept transfers from nearby devices or share a web link for easy downloads.';

  @override
  String get onboardingGetStarted => 'Get Started';

  @override
  String get onboardingSkip => 'Skip';

  @override
  String get onboardingNext => 'Next';

  @override
  String get unitBytes => 'B';

  @override
  String get unitKB => 'KB';

  @override
  String get unitMB => 'MB';

  @override
  String get unitGB => 'GB';

  @override
  String get bootingDaemon => 'Starting Lanos daemon...';

  @override
  String get bootFailed => 'Startup failed';

  @override
  String get retryBoot => 'Retry startup';

  @override
  String get refreshDevices => 'Refresh device list';

  @override
  String get nearbyDevices => 'Nearby devices';

  @override
  String get sendFile => 'Send file';

  @override
  String get searchingDevices => 'Searching nearby devices...';

  @override
  String get searchingDevicesHint =>
      'Make sure other devices have Lanos installed and are on the same Wi-Fi';

  @override
  String get selectDevice => 'Select a device';

  @override
  String get sendingStarted => 'Sending started';

  @override
  String sendFailed(Object error) {
    return 'Send failed: $error';
  }

  @override
  String get reconnectingGcd => 'Reconnecting to gcd...';

  @override
  String get transfers => 'Transfers';

  @override
  String get outgoing => 'Outgoing';

  @override
  String get incoming => 'Incoming';

  @override
  String get noTransferProgress => 'No transfer records';

  @override
  String get noTransferProgressHint =>
      'Send or receive files to see transfer progress and records here';

  @override
  String get receive => 'Receive';

  @override
  String get noReceiveRequests => 'No receive requests';

  @override
  String get noReceiveRequestsHint =>
      'When other devices send you files, they will appear here';

  @override
  String get deleteConfirmTitle => 'Delete confirmation';

  @override
  String get csvObtained => 'CSV obtained, copy to file';

  @override
  String get close => 'Close';

  @override
  String exportFailed(Object error) {
    return 'Export failed: $error';
  }

  @override
  String get exitSelect => 'Exit select';

  @override
  String get multiSelect => 'Multi-select';

  @override
  String get exportTransfersCSV => 'Export transfers CSV';

  @override
  String get exportSharesCSV => 'Export shares CSV';

  @override
  String get searchHint => 'Search by file name...';

  @override
  String searchQueryHint(Object query) {
    return 'Search: $query (requires backend API)';
  }

  @override
  String downloadsCount(Object count) {
    return '$count downloads';
  }

  @override
  String minutesAgo(Object count) {
    return '$count minutes ago';
  }

  @override
  String hoursAgo(Object count) {
    return '$count hours ago';
  }

  @override
  String daysAgo(Object count) {
    return '$count days ago';
  }

  @override
  String monthDay(Object day, Object month) {
    return '$month/$day';
  }

  @override
  String get statusWaiting => 'Waiting';

  @override
  String get statusTransferring => 'Transferring';

  @override
  String loadSettingsFailed(Object error) {
    return 'Failed to load settings: $error';
  }

  @override
  String get settingsSaved => 'Settings saved';

  @override
  String get saveSettings => 'Save settings';

  @override
  String saveFailed(Object error) {
    return 'Failed to save: $error';
  }

  @override
  String get sectionDevice => 'Device';

  @override
  String get sectionTransfer => 'Transfer';

  @override
  String get sectionNetwork => 'Network';

  @override
  String get sectionNotify => 'Notifications';

  @override
  String get sectionStorage => 'Storage';

  @override
  String get sectionAbout => 'About';

  @override
  String recordsN(Object count) {
    return '$count records';
  }

  @override
  String versionValue(Object version) {
    return 'Version $version';
  }

  @override
  String get cancelTooltip => 'Cancel';

  @override
  String get sasConfirmTitle => 'Confirm pairing';

  @override
  String sasVerifyPrompt(Object name) {
    return 'Please verify the following digits match \"$name\".';
  }

  @override
  String get sasVerifyHint =>
      'After they match, tap \"Confirm\" to establish the encrypted connection.';

  @override
  String sasAutoCancel(Object seconds) {
    return 'Auto-cancel in ${seconds}s';
  }

  @override
  String get thisDevice => 'This device';
}
