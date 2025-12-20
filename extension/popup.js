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
        statusDiv.textContent = "Sending to Spice...";
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
                    statusDiv.textContent = "Added to Spice!";
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
                statusDiv.textContent = "Error: " + err.message;
                statusDiv.style.color = "red";
            });
    }
});
