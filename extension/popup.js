document.addEventListener('DOMContentLoaded', () => {
    const contentFoundDiv = document.getElementById('content_found');
    const noContentDiv = document.getElementById('no_content');
    const urlDisplay = document.getElementById('url_display');
    const addBtn = document.getElementById('add_btn');
    const statusDiv = document.getElementById('status');

    const errorDiv = document.getElementById('content_error');

    // Initial State: Hide everything to prevent flashing
    contentFoundDiv.classList.add('hidden');
    noContentDiv.classList.add('hidden');
    errorDiv.classList.add('hidden');

    // Localize UI
    chrome.storage.local.get(['app_language'], (res) => {
        const appLang = res.app_language;
        if (appLang && appLang !== 'en') {
            // Manual override for localized UI
            fetch(`_locales/${appLang}/messages.json`)
                .then(r => r.json())
                .then(messages => {
                    document.querySelectorAll('[data-i18n]').forEach(el => {
                        const key = el.getAttribute('data-i18n');
                        if (messages[key]) {
                            el.textContent = messages[key].message;
                        }
                    });
                    // Store for functions
                    window.spiceLocales = messages;
                })
                .catch(err => {
                    console.error("Failed to load local messages:", err);
                    defaultLocalize();
                });
        } else {
            defaultLocalize();
        }
    });

    function defaultLocalize() {
        document.querySelectorAll('[data-i18n]').forEach(el => {
            const key = el.getAttribute('data-i18n');
            const message = chrome.i18n.getMessage(key);
            if (message) {
                el.textContent = message;
            }
        });
    }

    function getMsg(key) {
        if (window.spiceLocales && window.spiceLocales[key]) {
            return window.spiceLocales[key].message;
        }
        return chrome.i18n.getMessage(key);
    }

    // Check Backend Connection via Background Script with Timeout
    let responseReceived = false;
    const timeoutMsg = setTimeout(() => {
        if (!responseReceived) {
            console.warn("Background script timed out.");
            showError();
        }
    }, 500);

    chrome.runtime.sendMessage({ type: "GET_STATUS" }, (response) => {
        responseReceived = true;
        clearTimeout(timeoutMsg);

        if (chrome.runtime.lastError) {
            console.error(chrome.runtime.lastError);
            showError();
            return;
        }

        if (response && response.connected) {
            checkSource();
        } else {
            showError();
        }
    });

    function checkSource() {
        chrome.storage.local.get(['detected_source'], (result) => {
            if (result.detected_source) {
                contentFoundDiv.classList.remove('hidden');
                noContentDiv.classList.add('hidden');
                errorDiv.classList.add('hidden');
                urlDisplay.textContent = result.detected_source;

                addBtn.onclick = () => {
                    addQuery(result.detected_source);
                };
            } else {
                contentFoundDiv.classList.add('hidden');
                noContentDiv.classList.remove('hidden');
                errorDiv.classList.add('hidden');
            }
        });
    }

    function showError() {
        contentFoundDiv.classList.add('hidden');
        noContentDiv.classList.add('hidden');
        errorDiv.classList.remove('hidden');
    }

    function addQuery(url) {
        statusDiv.textContent = getMsg("sendingToSpice");
        statusDiv.style.color = "#8D6E63"; // Reset color

        fetch('http://127.0.0.1:49452/add', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ url: url })
        })
            .then(response => {
                if (response.ok) {
                    statusDiv.textContent = getMsg("addedToSpice");
                    statusDiv.style.color = "green";
                    // Disable button to prevent double-click
                    addBtn.disabled = true;
                    addBtn.style.opacity = "0.5";
                } else {
                    return response.text().then(text => { throw new Error(text) });
                }
            })
            .catch(err => {
                console.error(err);
                statusDiv.textContent = getMsg("errorPrefix") + err.message;
                statusDiv.style.color = "red";
            });
    }
});
