/*
    Localization
*/
let languages = ['en', 'zh'];
let languageNames = {
    'en': 'English',
    'zh': '中文（正體）'
};
let currentLanguage = 'en';

//Initialize the i18n dom library
var i18n = domI18n({
    selector: '[i18n]',
    separator: ' // ',
    languages: languages,
    defaultLanguage: 'en'
});

$(document).ready(function(){
    let userLang = navigator.language || navigator.userLanguage;
    console.log("User language: " + userLang);
    userLang = userLang.split("-")[0];
    if (!languages.includes(userLang)) {
        userLang = 'en';
    }
    i18n.changeLanguage(userLang);
    currentLanguage = userLang;
});

// Update language on newly loaded content
function relocale(){
    i18n.changeLanguage(currentLanguage);
}

function setCurrentLanguage(newLanguage){
    let languageName = languageNames[newLanguage];
    currentLanguage = newLanguage;
    $("#currentLanguage").html(languageName);
    i18n.changeLanguage(newLanguage);
}

/* Other Translated messages */
function i18nc(key, language=undefined){
   if (language === undefined){
       language = currentLanguage;
    }

    let translatedMessage = translatedMessages[language][key];
    if (translatedMessage === undefined){
        translatedMessage = translatedMessages['en'][key];
    }
    if (translatedMessage === undefined){
        translatedMessage = key;
    }
    return translatedMessage;
}

let translatedMessages = {
    'en': {
        'disk_info_refreshed': 'Disk information reloaded',
        "raid_resync_started_succ": 'RAID resync started',
        "raid_device_updated_succ": 'RAID device status reloaded',
        "raid_reassemble_started_succ": 'RAID config reloaded',
        "raid_device_deleted_succ": 'RAID device deleted',
        "raid_device_deleted_fail": 'RAID device delete failed',
        "raid_device_created_succ": 'RAID device created',
        "raid_device_created_fail": 'RAID device create failed',
        "raid_disk_failed_succ": 'Disk marked as failed',
        "raid_disk_removed_succ": 'Disk removed from array',
        "raid_disk_added_succ": 'Disk added to array, rebuild will start automatically',
        "raid_grow_succ": 'Array expansion started',
        "netmount_created_succ": 'Network filesystem mounted',
        "netmount_unmounted_succ": 'Network filesystem unmounted',
        "netmount_mounted_succ": 'Network filesystem mounted',
        "netmount_removed_succ": 'Connection removed',
        "netmount_updated_succ": 'Connection updated',
        "disk_mounted_succ": 'Partition mounted',
        "disk_unmounted_succ": 'Partition unmounted',
        "folder_created_succ": 'Folder created',
        "parttool_confirm_required": 'Please confirm you understand the data loss risk',
        "parttool_invalid_size": 'Invalid partition size',
        "parttool_working": 'Working on the disk, please wait...',
        "parttool_table_created_succ": 'Partition table created',
        "parttool_part_created_succ": 'Partition created',
        "parttool_part_deleted_succ": 'Partition deleted',
        "parttool_formatted_succ": 'Partition formatted',
        "parttool_parts_deleted_succ": 'Selected partitions deleted',
        "parttool_extended_succ": 'Partition extended',
        "parttool_recreated_succ": 'Disk recreated with new layout',
        "settings_saved_succ": 'Settings saved',
        "settings_test_sent_succ": 'Test alert delivered',
        "log_deleted_succ": 'Log file deleted',
    },
    'zh': {
        'disk_info_refreshed': '磁碟資訊已重新載入',
        "raid_resync_started_succ": 'RAID 重建已成功啟動',
        "raid_device_updated_succ": 'RAID 裝置狀態已重新載入',
        "raid_reassemble_started_succ": 'RAID 配置已重新載入',
        "raid_device_deleted_succ": 'RAID 裝置已刪除',
        "raid_device_deleted_fail": 'RAID 裝置刪除失敗',
        "raid_device_created_succ": 'RAID 裝置已建立',
        "raid_device_created_fail": 'RAID 裝置建立失敗',
        "raid_disk_failed_succ": '磁碟已標記為故障',
        "raid_disk_removed_succ": '磁碟已從陣列中移除',
        "raid_disk_added_succ": '磁碟已加入陣列，重建將自動開始',
        "raid_grow_succ": '陣列擴充已開始',
        "netmount_created_succ": '網路檔案系統已掛載',
        "netmount_unmounted_succ": '網路檔案系統已卸載',
        "netmount_mounted_succ": '網路檔案系統已掛載',
        "netmount_removed_succ": '連線已移除',
        "netmount_updated_succ": '連線已更新',
        "disk_mounted_succ": '分割區已掛載',
        "disk_unmounted_succ": '分割區已卸載',
        "folder_created_succ": '資料夾已建立',
        "parttool_confirm_required": '請確認您了解資料遺失的風險',
        "parttool_invalid_size": '無效的分割區大小',
        "parttool_working": '正在處理磁碟，請稍候...',
        "parttool_table_created_succ": '分割表已建立',
        "parttool_part_created_succ": '分割區已建立',
        "parttool_part_deleted_succ": '分割區已刪除',
        "parttool_formatted_succ": '分割區已格式化',
        "parttool_parts_deleted_succ": '所選分割區已刪除',
        "parttool_extended_succ": '分割區已擴充',
        "parttool_recreated_succ": '磁碟已以新配置重建',
        "settings_saved_succ": '設定已儲存',
        "settings_test_sent_succ": '測試警報已發送',
        "log_deleted_succ": '日誌檔案已刪除',
    }
};