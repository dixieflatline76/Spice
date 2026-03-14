function show(platform, enabled, useSettingsInsteadOfPreferences) {
    document.body.classList.add(`platform-${platform}`);

    if (useSettingsInsteadOfPreferences) {
        const lang = (navigator.language || 'en').split('-')[0];
        const messages = {
            'de': {
                'on': "Die Erweiterung des Spice Wallpaper Managers ist derzeit aktiviert. Sie können sie im Bereich Erweiterungen der Safari-Einstellungen deaktivieren.",
                'off': "Die Erweiterung des Spice Wallpaper Managers ist derzeit deaktiviert. Sie können sie im Bereich Erweiterungen der Safari-Einstellungen aktivieren.",
                'unknown': "Sie können die Erweiterung des Spice Wallpaper Managers im Bereich Erweiterungen der Safari-Einstellungen aktivieren.",
                'btn': "Beenden und Safari-Einstellungen öffnen…"
            },
            'es': {
                'on': "La extensión de Spice Wallpaper Manager está actualmente activada. Puedes desactivarla en la sección Extensiones de los ajustes de Safari.",
                'off': "La extensión de Spice Wallpaper Manager está actualmente desactivada. Puedes activarla en la sección Extensiones de los ajustes de Safari.",
                'unknown': "Puedes activar la extensión de Spice Wallpaper Manager en la sección Extensiones de los ajustes de Safari.",
                'btn': "Salir y abrir los ajustes de Safari…"
            },
            'fr': {
                'on': "L'extension Spice Wallpaper Manager est actuellement activée. Vous pouvez la désactiver dans la section Extensions des réglages de Safari.",
                'off': "L'extension Spice Wallpaper Manager est actuellement désactivée. Vous pouvez l'activer dans la section Extensions des réglages de Safari.",
                'unknown': "Vous pouvez activer l'extension Spice Wallpaper Manager dans la section Extensions des réglages de Safari.",
                'btn': "Quitter et ouvrir les réglages Safari…"
            },
            'it': {
                'on': "L'estensione di Spice Wallpaper Manager è attualmente attiva. Puoi disattivarla nella sezione Estensioni delle impostazioni di Safari.",
                'off': "L'estensione di Spice Wallpaper Manager è attualmente disattivata. Puoi attivarla nella sezione Estensioni delle impostazioni di Safari.",
                'unknown': "Puoi attivare l'estensione di Spice Wallpaper Manager nella sezione Estensioni delle impostazioni di Safari.",
                'btn': "Esci e apri le impostazioni di Safari…"
            },
            'ja': {
                'on': "Spice Wallpaper Manager 拡張機能は現在オンです。Safari の設定の「拡張機能」セクションでオフにすることができます。",
                'off': "Spice Wallpaper Manager 拡張機能は現在オフです。Safari の設定の「拡張機能」セクションでオンにすることができます。",
                'unknown': "Safari の設定の「拡張機能」セクションで Spice Wallpaper Manager 拡張機能をオンにすることができます。",
                'btn': "終了して Safari の設定を開く…"
            },
            'pt': {
                'on': "A extensão Spice Wallpaper Manager está atualmente ligada. Pode desligá-la na secção Extensões das definições do Safari.",
                'off': "A extensão Spice Wallpaper Manager está atualmente desligada. Pode ligá-la na secção Extensões das definições do Safari.",
                'unknown': "Pode ligar a extensão Spice Wallpaper Manager na secção Extensões das definições do Safari.",
                'btn': "Sair e abrir as definições do Safari…"
            },
            'ru': {
                'on': "Расширение Spice Wallpaper Manager включено. Его можно выключить в разделе «Расширения» в настройках Safari.",
                'off': "Расширение Spice Wallpaper Manager выключено. Его можно включить в разделе «Расширения» в настройках Safari.",
                'unknown': "Вы можете включить расширение Spice Wallpaper Manager в разделе «Расширения» в настройках Safari.",
                'btn': "Завершить и открыть настройки Safari…"
            },
            'uk': {
                'on': "Розширення Spice Wallpaper Manager увімкнено. Його можна вимкнути в розділі «Розширення» в налаштуваннях Safari.",
                'off': "Розширення Spice Wallpaper Manager вимкнено. Його можна увімкнути в розділі «Розширення» в налаштуваннях Safari.",
                'unknown': "Ви можете увімкнути розширення Spice Wallpaper Manager у розділі «Розширення» в налаштуваннях Safari.",
                'btn': "Завершити та відкрити налаштування Safari…"
            },
            'zh': {
                'on': "Spice Wallpaper Manager 扩展当前已开启。您可以在 Safari 设置的“扩展”部分将其关闭。",
                'off': "Spice Wallpaper Manager 扩展当前已关闭。您可以在 Safari 设置的“扩展”部分将其开启。",
                'unknown': "您可以在 Safari 设置的“扩展”部分开启 Spice Wallpaper Manager 扩展。",
                'btn': "推出并打开 Safari 设置…"
            },
            'zh-tw': {
                'on': "Spice Wallpaper Manager 擴充功能目前已開啟。您可以在 Safari 設定的「擴充功能」部分將其關閉。",
                'off': "Spice Wallpaper Manager 擴充功能目前已關閉。您可以在 Safari 設定的「擴充功能」部分將其開啟。",
                'unknown': "您可以在 Safari 設定的「擴充功能」部分開啟 Spice Wallpaper Manager 擴充功能。",
                'btn': "結束並開啟 Safari 設定…"
            },
            'en': {
                'on': "Spice Wallpaper Manager Extension’s extension is currently on. You can turn it off in the Extensions section of Safari Settings.",
                'off': "Spice Wallpaper Manager Extension’s extension is currently off. You can turn it on in the Extensions section of Safari Settings.",
                'unknown': "You can turn on Spice Wallpaper Manager Extension’s extension in the Extensions section of Safari Settings.",
                'btn': "Quit and Open Safari Settings…"
            }
        };

        const msg = messages[lang] || messages['en'];
        document.getElementsByClassName('platform-mac state-on')[0].innerText = msg.on;
        document.getElementsByClassName('platform-mac state-off')[0].innerText = msg.off;
        document.getElementsByClassName('platform-mac state-unknown')[0].innerText = msg.unknown;
        document.getElementsByClassName('platform-mac open-preferences')[0].innerText = msg.btn;
    }

    if (typeof enabled === "boolean") {
        document.body.classList.toggle(`state-on`, enabled);
        document.body.classList.toggle(`state-off`, !enabled);
    } else {
        document.body.classList.remove(`state-on`);
        document.body.classList.remove(`state-off`);
    }
}

function openPreferences() {
    webkit.messageHandlers.controller.postMessage("open-preferences");
}

document.querySelector("button.open-preferences").addEventListener("click", openPreferences);
