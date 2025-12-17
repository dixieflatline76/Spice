// Spice Background Service Worker (Verified Keep-Alive Version)

let socket = null;
let keepAliveInterval = null;
let animationInterval = null;

const WS_URL = 'ws://127.0.0.1:49452/ws';
const HEALTH_URL = 'http://127.0.0.1:49452/health';

// Backoff Configuration
const INITIAL_RETRY_DELAY = 1000;
const MAX_RETRY_DELAY = 10000;
const BACKOFF_FACTOR = 1.5;

let retryDelay = INITIAL_RETRY_DELAY;
let isConnected = false;

// --- WebSocket Connection ---

function connect() {
    if (socket) return;

    // Only log intent if backoff is significant
    if (retryDelay >= 5000) {
        console.log(`Polling Spice Backend (next retry in ${Math.round(retryDelay / 1000)}s)...`);
    } else {
        console.log('Polling Spice Backend...');
    }

    fetch(HEALTH_URL)
        .then(response => {
            if (response.ok) {
                console.log('Spice Backend is UP. Opening WebSocket...');
                openSocket();
            } else {
                throw new Error('Health check failed: ' + response.statusText);
            }
        })
        .catch(err => {
            scheduleReconnect();
        });
}

function openSocket() {
    socket = new WebSocket(WS_URL);

    socket.onopen = () => {
        console.log('Connected to Spice Backend.');
        isConnected = true;
        retryDelay = INITIAL_RETRY_DELAY;
        startKeepAlive();
        // Reset icon to default (handled by URL check usually, but good safeguard)
        chrome.action.setIcon({ path: "icon_128.png" });
    };

    socket.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        // Handle incoming messages if any (e.g. "Scan Connected")
        handleMessage(msg);
    };

    socket.onclose = () => {
        if (isConnected) console.log('Disconnected from Spice Backend.');
        isConnected = false;
        socket = null;
        stopKeepAlive();
        scheduleReconnect();
    };

    socket.onerror = (err) => {
        // Suppress errors during connection attempts
    };
}

function scheduleReconnect() {
    setTimeout(connect, retryDelay);
    retryDelay = Math.min(retryDelay * BACKOFF_FACTOR, MAX_RETRY_DELAY);
}

// --- Keep-Alive / Alarms ---

// Wake up every 30s to prevent Service Worker death
chrome.alarms.create('keepAlive', { periodInMinutes: 0.5 });

chrome.alarms.onAlarm.addListener((alarm) => {
    if (alarm.name === 'keepAlive') {
        // Ping socket to keep connection open
        if (socket && socket.readyState === WebSocket.OPEN) {
            socket.send(JSON.stringify({ type: 'ping' }));
        } else if (!socket) {
            connect(); // Reconnect if somehow dead
        }
    }
});

function startKeepAlive() {
    stopKeepAlive();
    // Also use JS interval as backup for active sessions
    keepAliveInterval = setInterval(() => {
        if (socket && socket.readyState === WebSocket.OPEN) {
            socket.send(JSON.stringify({ type: 'ping' }));
        }
    }, 20000);
}

function stopKeepAlive() {
    if (keepAliveInterval) clearInterval(keepAliveInterval);
    keepAliveInterval = null;
}

// --- Message Handling ---

async function handleMessage(msg) {
    if (msg.type === 'set_wallpaper') {
        console.warn("set_wallpaper command received but feature is currently PAUSED.");
    }
}

// --- Supported Sites & Animation ---

const SUPPORTED_PATTERNS = [
    // Wallhaven
    /^https:\/\/wallhaven\.cc\/(?:latest|toplist|hot|random|search|api\/v1\/search|api\/v1\/collections\/[a-zA-Z0-9_]+\/[0-9]+|user\/[a-zA-Z0-9_]+\/favorites\/[0-9]+|favorites\/[0-9]+)(?:\?[a-zA-Z0-9_\-.~!$&'()*+,;=:@\/?%]*|)$/,
    // Pexels
    /^https:\/\/(?:www\.)?pexels\.com\/(?:search\/|collections\/).*$/,
    // Unsplash
    /^https:\/\/(?:www\.)?unsplash\.com\/(?:collections\/|s\/photos\/).*$/,
    // Wikimedia
    /^https:\/\/commons\.wikimedia\.org\/(?:wiki\/Category:|w\/index\.php\?).*$/
];

function startAnimation() {
    if (animationInterval) return;
    let frame = 0;
    chrome.action.setIcon({ path: "icon_anim_128.png" }); // Immediate
    animationInterval = setInterval(() => {
        frame = (frame + 1) % 2;
        const iconPath = frame === 0 ? "icon_128.png" : "icon_anim_128.png";
        chrome.action.setIcon({ path: iconPath });
    }, 500);
}

function stopAnimation() {
    if (animationInterval) clearInterval(animationInterval);
    animationInterval = null;
    chrome.action.setIcon({ path: "icon_128.png" });
}

function checkAndStoreUrl(url) {
    if (!url) {
        chrome.storage.local.remove("detected_source");
        stopAnimation();
        return;
    }
    const isSupported = SUPPORTED_PATTERNS.some(regex => regex.test(url));
    if (isSupported) {
        chrome.storage.local.set({ "detected_source": url });
        startAnimation();
    } else {
        chrome.storage.local.remove("detected_source");
        stopAnimation();
    }
}

// --- Event Listeners ---

chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
    if (changeInfo.status === 'complete' && tab.url) {
        checkAndStoreUrl(tab.url);
    }
});

chrome.tabs.onActivated.addListener((activeInfo) => {
    chrome.tabs.get(activeInfo.tabId, (tab) => {
        if (chrome.runtime.lastError) return;
        checkAndStoreUrl(tab.url);
    });
});

chrome.windows.onFocusChanged.addListener((windowId) => {
    if (windowId === chrome.windows.WINDOW_ID_NONE) return;
    chrome.tabs.query({ active: true, windowId: windowId }, (tabs) => {
        if (tabs && tabs.length > 0) {
            checkAndStoreUrl(tabs[0].url);
        }
    });
});

// Communication with Popup (Instant Feedback)
chrome.runtime.onMessage.addListener((request, sender, sendResponse) => {
    if (request.type === "GET_STATUS") {
        sendResponse({ connected: isConnected });
    }
});

// Initial Startup
connect();
chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
    if (tabs && tabs.length > 0) {
        checkAndStoreUrl(tabs[0].url);
    }
});
