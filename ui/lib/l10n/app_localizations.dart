import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/intl.dart' as intl;

import 'app_localizations_en.dart';
import 'app_localizations_zh.dart';

// ignore_for_file: type=lint

/// Callers can lookup localized strings with an instance of AppLocalizations
/// returned by `AppLocalizations.of(context)`.
///
/// Applications need to include `AppLocalizations.delegate()` in their app's
/// `localizationDelegates` list, and the locales they support in the app's
/// `supportedLocales` list. For example:
///
/// ```dart
/// import 'l10n/app_localizations.dart';
///
/// return MaterialApp(
///   localizationsDelegates: AppLocalizations.localizationsDelegates,
///   supportedLocales: AppLocalizations.supportedLocales,
///   home: MyApplicationHome(),
/// );
/// ```
///
/// ## Update pubspec.yaml
///
/// Please make sure to update your pubspec.yaml to include the following
/// packages:
///
/// ```yaml
/// dependencies:
///   # Internationalization support.
///   flutter_localizations:
///     sdk: flutter
///   intl: any # Use the pinned version from flutter_localizations
///
///   # Rest of dependencies
/// ```
///
/// ## iOS Applications
///
/// iOS applications define key application metadata, including supported
/// locales, in an Info.plist file that is built into the application bundle.
/// To configure the locales supported by your app, you’ll need to edit this
/// file.
///
/// First, open your project’s ios/Runner.xcworkspace Xcode workspace file.
/// Then, in the Project Navigator, open the Info.plist file under the Runner
/// project’s Runner folder.
///
/// Next, select the Information Property List item, select Add Item from the
/// Editor menu, then select Localizations from the pop-up menu.
///
/// Select and expand the newly-created Localizations item then, for each
/// locale your application supports, add a new item and select the locale
/// you wish to add from the pop-up menu in the Value field. This list should
/// be consistent with the languages listed in the AppLocalizations.supportedLocales
/// property.
abstract class AppLocalizations {
  AppLocalizations(String locale)
      : localeName = intl.Intl.canonicalizedLocale(locale.toString());

  final String localeName;

  static AppLocalizations? of(BuildContext context) {
    return Localizations.of<AppLocalizations>(context, AppLocalizations);
  }

  static const LocalizationsDelegate<AppLocalizations> delegate =
      _AppLocalizationsDelegate();

  /// A list of this localizations delegate along with the default localizations
  /// delegates.
  ///
  /// Returns a list of localizations delegates containing this delegate along with
  /// GlobalMaterialLocalizations.delegate, GlobalCupertinoLocalizations.delegate,
  /// and GlobalWidgetsLocalizations.delegate.
  ///
  /// Additional delegates can be added by appending to this list in
  /// MaterialApp. This list does not have to be used at all if a custom list
  /// of delegates is preferred or required.
  static const List<LocalizationsDelegate<dynamic>> localizationsDelegates =
      <LocalizationsDelegate<dynamic>>[
    delegate,
    GlobalMaterialLocalizations.delegate,
    GlobalCupertinoLocalizations.delegate,
    GlobalWidgetsLocalizations.delegate,
  ];

  /// A list of this localizations delegate's supported locales.
  static const List<Locale> supportedLocales = <Locale>[
    Locale('en'),
    Locale('zh')
  ];

  /// Application title shown in app bar
  ///
  /// In en, this message translates to:
  /// **'Lanos'**
  String get appTitle;

  /// No description provided for @home.
  ///
  /// In en, this message translates to:
  /// **'Home'**
  String get home;

  /// No description provided for @records.
  ///
  /// In en, this message translates to:
  /// **'Records'**
  String get records;

  /// No description provided for @settings.
  ///
  /// In en, this message translates to:
  /// **'Settings'**
  String get settings;

  /// No description provided for @transferRecords.
  ///
  /// In en, this message translates to:
  /// **'Transfers'**
  String get transferRecords;

  /// No description provided for @shareRecords.
  ///
  /// In en, this message translates to:
  /// **'Shares'**
  String get shareRecords;

  /// No description provided for @noTransferRecords.
  ///
  /// In en, this message translates to:
  /// **'No transfer records'**
  String get noTransferRecords;

  /// No description provided for @noTransferRecordsHint.
  ///
  /// In en, this message translates to:
  /// **'Send or receive files to see them here'**
  String get noTransferRecordsHint;

  /// No description provided for @noShareRecords.
  ///
  /// In en, this message translates to:
  /// **'No share records'**
  String get noShareRecords;

  /// No description provided for @noShareRecordsHint.
  ///
  /// In en, this message translates to:
  /// **'Share files via web link to see them here'**
  String get noShareRecordsHint;

  /// No description provided for @search.
  ///
  /// In en, this message translates to:
  /// **'Search'**
  String get search;

  /// No description provided for @sort.
  ///
  /// In en, this message translates to:
  /// **'Sort'**
  String get sort;

  /// No description provided for @sortByTime.
  ///
  /// In en, this message translates to:
  /// **'Time'**
  String get sortByTime;

  /// No description provided for @sortBySize.
  ///
  /// In en, this message translates to:
  /// **'Size'**
  String get sortBySize;

  /// No description provided for @sortByName.
  ///
  /// In en, this message translates to:
  /// **'Name'**
  String get sortByName;

  /// No description provided for @sortByStatus.
  ///
  /// In en, this message translates to:
  /// **'Status'**
  String get sortByStatus;

  /// No description provided for @ascending.
  ///
  /// In en, this message translates to:
  /// **'Ascending'**
  String get ascending;

  /// No description provided for @descending.
  ///
  /// In en, this message translates to:
  /// **'Descending'**
  String get descending;

  /// No description provided for @selectAll.
  ///
  /// In en, this message translates to:
  /// **'Select All'**
  String get selectAll;

  /// No description provided for @delete.
  ///
  /// In en, this message translates to:
  /// **'Delete'**
  String get delete;

  /// No description provided for @deleteSelected.
  ///
  /// In en, this message translates to:
  /// **'Delete selected'**
  String get deleteSelected;

  /// No description provided for @deleteConfirm.
  ///
  /// In en, this message translates to:
  /// **'Are you sure you want to delete {count} record(s)?'**
  String deleteConfirm(Object count);

  /// No description provided for @cancel.
  ///
  /// In en, this message translates to:
  /// **'Cancel'**
  String get cancel;

  /// No description provided for @confirm.
  ///
  /// In en, this message translates to:
  /// **'Confirm'**
  String get confirm;

  /// No description provided for @exportCSV.
  ///
  /// In en, this message translates to:
  /// **'Export CSV'**
  String get exportCSV;

  /// No description provided for @statusCompleted.
  ///
  /// In en, this message translates to:
  /// **'Completed'**
  String get statusCompleted;

  /// No description provided for @statusFailed.
  ///
  /// In en, this message translates to:
  /// **'Failed'**
  String get statusFailed;

  /// No description provided for @statusCancelled.
  ///
  /// In en, this message translates to:
  /// **'Cancelled'**
  String get statusCancelled;

  /// No description provided for @statusActive.
  ///
  /// In en, this message translates to:
  /// **'Active'**
  String get statusActive;

  /// No description provided for @statusExpired.
  ///
  /// In en, this message translates to:
  /// **'Expired'**
  String get statusExpired;

  /// No description provided for @statusStopped.
  ///
  /// In en, this message translates to:
  /// **'Stopped'**
  String get statusStopped;

  /// No description provided for @statusPending.
  ///
  /// In en, this message translates to:
  /// **'Pending'**
  String get statusPending;

  /// No description provided for @directionSend.
  ///
  /// In en, this message translates to:
  /// **'Sent'**
  String get directionSend;

  /// No description provided for @directionReceive.
  ///
  /// In en, this message translates to:
  /// **'Received'**
  String get directionReceive;

  /// No description provided for @sizePrefix.
  ///
  /// In en, this message translates to:
  /// **'Size: {size}'**
  String sizePrefix(Object size);

  /// No description provided for @timePrefix.
  ///
  /// In en, this message translates to:
  /// **'Time: {time}'**
  String timePrefix(Object time);

  /// No description provided for @peerPrefix.
  ///
  /// In en, this message translates to:
  /// **'Peer: {name}'**
  String peerPrefix(Object name);

  /// No description provided for @deviceName.
  ///
  /// In en, this message translates to:
  /// **'Device Name'**
  String get deviceName;

  /// No description provided for @downloadPath.
  ///
  /// In en, this message translates to:
  /// **'Download Path'**
  String get downloadPath;

  /// No description provided for @autoReceive.
  ///
  /// In en, this message translates to:
  /// **'Auto Receive'**
  String get autoReceive;

  /// No description provided for @autoReceiveAsk.
  ///
  /// In en, this message translates to:
  /// **'Ask'**
  String get autoReceiveAsk;

  /// No description provided for @autoReceiveTrusted.
  ///
  /// In en, this message translates to:
  /// **'Trusted devices only'**
  String get autoReceiveTrusted;

  /// No description provided for @autoReceiveAll.
  ///
  /// In en, this message translates to:
  /// **'All devices'**
  String get autoReceiveAll;

  /// No description provided for @conflictPolicy.
  ///
  /// In en, this message translates to:
  /// **'File Conflict'**
  String get conflictPolicy;

  /// No description provided for @conflictPolicySkip.
  ///
  /// In en, this message translates to:
  /// **'Skip'**
  String get conflictPolicySkip;

  /// No description provided for @conflictPolicyOverwrite.
  ///
  /// In en, this message translates to:
  /// **'Overwrite'**
  String get conflictPolicyOverwrite;

  /// No description provided for @conflictPolicyKeepBoth.
  ///
  /// In en, this message translates to:
  /// **'Keep Both'**
  String get conflictPolicyKeepBoth;

  /// No description provided for @stealthMode.
  ///
  /// In en, this message translates to:
  /// **'Stealth Mode'**
  String get stealthMode;

  /// No description provided for @stealthModeHint.
  ///
  /// In en, this message translates to:
  /// **'Hide from other devices'**
  String get stealthModeHint;

  /// No description provided for @ipv6Preferred.
  ///
  /// In en, this message translates to:
  /// **'Prefer IPv6'**
  String get ipv6Preferred;

  /// No description provided for @ipv6PreferredHint.
  ///
  /// In en, this message translates to:
  /// **'Use IPv6 when available'**
  String get ipv6PreferredHint;

  /// No description provided for @notifyReceive.
  ///
  /// In en, this message translates to:
  /// **'Notify on Receive'**
  String get notifyReceive;

  /// No description provided for @notifySent.
  ///
  /// In en, this message translates to:
  /// **'Notify on Sent'**
  String get notifySent;

  /// No description provided for @notifyShare.
  ///
  /// In en, this message translates to:
  /// **'Notify on Share'**
  String get notifyShare;

  /// No description provided for @recordRetention.
  ///
  /// In en, this message translates to:
  /// **'Record Retention'**
  String get recordRetention;

  /// No description provided for @recordRetentionUnlimited.
  ///
  /// In en, this message translates to:
  /// **'Unlimited'**
  String get recordRetentionUnlimited;

  /// No description provided for @recordsCount.
  ///
  /// In en, this message translates to:
  /// **'{count} records'**
  String recordsCount(Object count);

  /// No description provided for @language.
  ///
  /// In en, this message translates to:
  /// **'Language'**
  String get language;

  /// No description provided for @languageSystem.
  ///
  /// In en, this message translates to:
  /// **'System default'**
  String get languageSystem;

  /// No description provided for @languageZh.
  ///
  /// In en, this message translates to:
  /// **'中文'**
  String get languageZh;

  /// No description provided for @languageEn.
  ///
  /// In en, this message translates to:
  /// **'English'**
  String get languageEn;

  /// No description provided for @about.
  ///
  /// In en, this message translates to:
  /// **'About'**
  String get about;

  /// No description provided for @version.
  ///
  /// In en, this message translates to:
  /// **'Version'**
  String get version;

  /// No description provided for @onboardingWelcome.
  ///
  /// In en, this message translates to:
  /// **'Welcome to Lanos'**
  String get onboardingWelcome;

  /// No description provided for @onboardingWelcomeDesc.
  ///
  /// In en, this message translates to:
  /// **'Share files securely over local network. No internet required, no size limits.'**
  String get onboardingWelcomeDesc;

  /// No description provided for @onboardingSend.
  ///
  /// In en, this message translates to:
  /// **'Send Files'**
  String get onboardingSend;

  /// No description provided for @onboardingSendDesc.
  ///
  /// In en, this message translates to:
  /// **'Select files, choose a device, and transfer with end-to-end encryption.'**
  String get onboardingSendDesc;

  /// No description provided for @onboardingReceive.
  ///
  /// In en, this message translates to:
  /// **'Receive Files'**
  String get onboardingReceive;

  /// No description provided for @onboardingReceiveDesc.
  ///
  /// In en, this message translates to:
  /// **'Accept transfers from nearby devices or share a web link for easy downloads.'**
  String get onboardingReceiveDesc;

  /// No description provided for @onboardingGetStarted.
  ///
  /// In en, this message translates to:
  /// **'Get Started'**
  String get onboardingGetStarted;

  /// No description provided for @onboardingSkip.
  ///
  /// In en, this message translates to:
  /// **'Skip'**
  String get onboardingSkip;

  /// No description provided for @onboardingNext.
  ///
  /// In en, this message translates to:
  /// **'Next'**
  String get onboardingNext;

  /// No description provided for @unitBytes.
  ///
  /// In en, this message translates to:
  /// **'B'**
  String get unitBytes;

  /// No description provided for @unitKB.
  ///
  /// In en, this message translates to:
  /// **'KB'**
  String get unitKB;

  /// No description provided for @unitMB.
  ///
  /// In en, this message translates to:
  /// **'MB'**
  String get unitMB;

  /// No description provided for @unitGB.
  ///
  /// In en, this message translates to:
  /// **'GB'**
  String get unitGB;

  /// No description provided for @bootingDaemon.
  ///
  /// In en, this message translates to:
  /// **'Starting Lanos daemon...'**
  String get bootingDaemon;

  /// No description provided for @bootFailed.
  ///
  /// In en, this message translates to:
  /// **'Startup failed'**
  String get bootFailed;

  /// No description provided for @retryBoot.
  ///
  /// In en, this message translates to:
  /// **'Retry startup'**
  String get retryBoot;

  /// No description provided for @refreshDevices.
  ///
  /// In en, this message translates to:
  /// **'Refresh device list'**
  String get refreshDevices;

  /// No description provided for @nearbyDevices.
  ///
  /// In en, this message translates to:
  /// **'Nearby devices'**
  String get nearbyDevices;

  /// No description provided for @sendFile.
  ///
  /// In en, this message translates to:
  /// **'Send file'**
  String get sendFile;

  /// No description provided for @searchingDevices.
  ///
  /// In en, this message translates to:
  /// **'Searching nearby devices...'**
  String get searchingDevices;

  /// No description provided for @searchingDevicesHint.
  ///
  /// In en, this message translates to:
  /// **'Make sure other devices have Lanos installed and are on the same Wi-Fi'**
  String get searchingDevicesHint;

  /// No description provided for @selectDevice.
  ///
  /// In en, this message translates to:
  /// **'Select a device'**
  String get selectDevice;

  /// No description provided for @sendingStarted.
  ///
  /// In en, this message translates to:
  /// **'Sending started'**
  String get sendingStarted;

  /// No description provided for @sendFailed.
  ///
  /// In en, this message translates to:
  /// **'Send failed: {error}'**
  String sendFailed(Object error);

  /// No description provided for @reconnectingGcd.
  ///
  /// In en, this message translates to:
  /// **'Reconnecting to gcd...'**
  String get reconnectingGcd;

  /// No description provided for @transfers.
  ///
  /// In en, this message translates to:
  /// **'Transfers'**
  String get transfers;

  /// No description provided for @outgoing.
  ///
  /// In en, this message translates to:
  /// **'Outgoing'**
  String get outgoing;

  /// No description provided for @incoming.
  ///
  /// In en, this message translates to:
  /// **'Incoming'**
  String get incoming;

  /// No description provided for @noTransferProgress.
  ///
  /// In en, this message translates to:
  /// **'No transfer records'**
  String get noTransferProgress;

  /// No description provided for @noTransferProgressHint.
  ///
  /// In en, this message translates to:
  /// **'Send or receive files to see transfer progress and records here'**
  String get noTransferProgressHint;

  /// No description provided for @receive.
  ///
  /// In en, this message translates to:
  /// **'Receive'**
  String get receive;

  /// No description provided for @noReceiveRequests.
  ///
  /// In en, this message translates to:
  /// **'No receive requests'**
  String get noReceiveRequests;

  /// No description provided for @noReceiveRequestsHint.
  ///
  /// In en, this message translates to:
  /// **'When other devices send you files, they will appear here'**
  String get noReceiveRequestsHint;

  /// No description provided for @deleteConfirmTitle.
  ///
  /// In en, this message translates to:
  /// **'Delete confirmation'**
  String get deleteConfirmTitle;

  /// No description provided for @csvObtained.
  ///
  /// In en, this message translates to:
  /// **'CSV obtained, copy to file'**
  String get csvObtained;

  /// No description provided for @close.
  ///
  /// In en, this message translates to:
  /// **'Close'**
  String get close;

  /// No description provided for @exportFailed.
  ///
  /// In en, this message translates to:
  /// **'Export failed: {error}'**
  String exportFailed(Object error);

  /// No description provided for @exitSelect.
  ///
  /// In en, this message translates to:
  /// **'Exit select'**
  String get exitSelect;

  /// No description provided for @multiSelect.
  ///
  /// In en, this message translates to:
  /// **'Multi-select'**
  String get multiSelect;

  /// No description provided for @exportTransfersCSV.
  ///
  /// In en, this message translates to:
  /// **'Export transfers CSV'**
  String get exportTransfersCSV;

  /// No description provided for @exportSharesCSV.
  ///
  /// In en, this message translates to:
  /// **'Export shares CSV'**
  String get exportSharesCSV;

  /// No description provided for @searchHint.
  ///
  /// In en, this message translates to:
  /// **'Search by file name...'**
  String get searchHint;

  /// No description provided for @searchQueryHint.
  ///
  /// In en, this message translates to:
  /// **'Search: {query} (requires backend API)'**
  String searchQueryHint(Object query);

  /// No description provided for @downloadsCount.
  ///
  /// In en, this message translates to:
  /// **'{count} downloads'**
  String downloadsCount(Object count);

  /// No description provided for @minutesAgo.
  ///
  /// In en, this message translates to:
  /// **'{count} minutes ago'**
  String minutesAgo(Object count);

  /// No description provided for @hoursAgo.
  ///
  /// In en, this message translates to:
  /// **'{count} hours ago'**
  String hoursAgo(Object count);

  /// No description provided for @daysAgo.
  ///
  /// In en, this message translates to:
  /// **'{count} days ago'**
  String daysAgo(Object count);

  /// No description provided for @monthDay.
  ///
  /// In en, this message translates to:
  /// **'{month}/{day}'**
  String monthDay(Object day, Object month);

  /// No description provided for @statusWaiting.
  ///
  /// In en, this message translates to:
  /// **'Waiting'**
  String get statusWaiting;

  /// No description provided for @statusTransferring.
  ///
  /// In en, this message translates to:
  /// **'Transferring'**
  String get statusTransferring;

  /// No description provided for @loadSettingsFailed.
  ///
  /// In en, this message translates to:
  /// **'Failed to load settings: {error}'**
  String loadSettingsFailed(Object error);

  /// No description provided for @settingsSaved.
  ///
  /// In en, this message translates to:
  /// **'Settings saved'**
  String get settingsSaved;

  /// No description provided for @saveSettings.
  ///
  /// In en, this message translates to:
  /// **'Save settings'**
  String get saveSettings;

  /// No description provided for @saveFailed.
  ///
  /// In en, this message translates to:
  /// **'Failed to save: {error}'**
  String saveFailed(Object error);

  /// No description provided for @sectionDevice.
  ///
  /// In en, this message translates to:
  /// **'Device'**
  String get sectionDevice;

  /// No description provided for @sectionTransfer.
  ///
  /// In en, this message translates to:
  /// **'Transfer'**
  String get sectionTransfer;

  /// No description provided for @sectionNetwork.
  ///
  /// In en, this message translates to:
  /// **'Network'**
  String get sectionNetwork;

  /// No description provided for @sectionNotify.
  ///
  /// In en, this message translates to:
  /// **'Notifications'**
  String get sectionNotify;

  /// No description provided for @sectionStorage.
  ///
  /// In en, this message translates to:
  /// **'Storage'**
  String get sectionStorage;

  /// No description provided for @sectionAbout.
  ///
  /// In en, this message translates to:
  /// **'About'**
  String get sectionAbout;

  /// No description provided for @recordsN.
  ///
  /// In en, this message translates to:
  /// **'{count} records'**
  String recordsN(Object count);

  /// No description provided for @versionValue.
  ///
  /// In en, this message translates to:
  /// **'Version {version}'**
  String versionValue(Object version);

  /// No description provided for @cancelTooltip.
  ///
  /// In en, this message translates to:
  /// **'Cancel'**
  String get cancelTooltip;

  /// No description provided for @sasConfirmTitle.
  ///
  /// In en, this message translates to:
  /// **'Confirm pairing'**
  String get sasConfirmTitle;

  /// No description provided for @sasVerifyPrompt.
  ///
  /// In en, this message translates to:
  /// **'Please verify the following digits match \"{name}\".'**
  String sasVerifyPrompt(Object name);

  /// No description provided for @sasVerifyHint.
  ///
  /// In en, this message translates to:
  /// **'After they match, tap \"Confirm\" to establish the encrypted connection.'**
  String get sasVerifyHint;

  /// No description provided for @sasAutoCancel.
  ///
  /// In en, this message translates to:
  /// **'Auto-cancel in {seconds}s'**
  String sasAutoCancel(Object seconds);

  /// No description provided for @thisDevice.
  ///
  /// In en, this message translates to:
  /// **'This device'**
  String get thisDevice;
}

class _AppLocalizationsDelegate
    extends LocalizationsDelegate<AppLocalizations> {
  const _AppLocalizationsDelegate();

  @override
  Future<AppLocalizations> load(Locale locale) {
    return SynchronousFuture<AppLocalizations>(lookupAppLocalizations(locale));
  }

  @override
  bool isSupported(Locale locale) =>
      <String>['en', 'zh'].contains(locale.languageCode);

  @override
  bool shouldReload(_AppLocalizationsDelegate old) => false;
}

AppLocalizations lookupAppLocalizations(Locale locale) {
  // Lookup logic when only language code is specified.
  switch (locale.languageCode) {
    case 'en':
      return AppLocalizationsEn();
    case 'zh':
      return AppLocalizationsZh();
  }

  throw FlutterError(
      'AppLocalizations.delegate failed to load unsupported locale "$locale". This is likely '
      'an issue with the localizations generation tool. Please file an issue '
      'on GitHub with a reproducible sample app and the gen-l10n configuration '
      'that was used.');
}
