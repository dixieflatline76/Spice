// Spice Background Service Worker

let socket = null;
let keepAliveInterval = null;
let animationInterval = null;
// --- WebSocket Connection ---

const WS_URL = 'ws://127.0.0.1:49452/ws';
const HEALTH_URL = 'http://127.0.0.1:49452/health';

// Backoff Configuration
const INITIAL_RETRY_DELAY = 1000; // 1 second (snappy start)
const MAX_RETRY_DELAY = 10000;    // 10 seconds (aggressive cap)
const BACKOFF_FACTOR = 1.5;       // Gentler ramp-up

let retryDelay = INITIAL_RETRY_DELAY;
let isConnected = false;

function connect() {
    if (socket) return;

    // Only log intent if backoff is significant, to reduce noise
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
            // Backend offline. Silent fail (or log debug).
            // Exponential Backoff for next POLL
            scheduleReconnect();
        });
}

function openSocket() {
    socket = new WebSocket(WS_URL);

    socket.onopen = () => {
        console.log('Connected to Spice Backend.');
        isConnected = true;
        retryDelay = INITIAL_RETRY_DELAY; // Reset backoff
        startKeepAlive();
        chrome.action.setIcon({ path: "/icon_128.png" }); // Reset icon
    };

    socket.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        console.log('Received:', msg);
        handleMessage(msg);
    };

    socket.onclose = () => {
        if (isConnected) {
            console.log('Disconnected from Spice Backend.');
        }
        isConnected = false;
        socket = null;
        stopKeepAlive();
        // Immediately try to reconnect (which will start polling again)
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

function startKeepAlive() {
    stopKeepAlive();
    keepAliveInterval = setInterval(() => {
        if (socket && socket.readyState === WebSocket.OPEN) {
            socket.send(JSON.stringify({ type: 'ping' }));
        }
    }, 20000); // 20s
}

function stopKeepAlive() {
    if (keepAliveInterval) clearInterval(keepAliveInterval);
}

// --- Message Handling ---

function handleMessage(msg) {
    if (msg.type === 'set_wallpaper') {
        // Check if Chrome OS API is available
        if (chrome.wallpaper) {
            // According to Chrome docs, setWallpaper takes binary data or URL.
            // If 'url' is a local file path (from Go), Chrome Extension CANNOT access it directly 
            // unless we serve it via HTTP or pass base64.
            // However, the 'url' sent by server is a local path e.g. /home/user/img.jpg.
            // On ChromeOS (Crostini), the extension runs in Ash, Go runs in Linux container.
            // They might share files via "Linux Files".
            // BUT chrome.wallpaper.setWallpaper expects binary or url.
            // If we send a URL, chrome will download it.
            // If Go serves the file via HTTP, we can pass that URL.
            // HACK: For now, assuming we might need to change Backend to Serve the file?
            // OR: We send the command, but realize wait...
            // chrome.wallpaper.setWallpaper is for Setting the Wallpaper of CHROME OS.
            // It accepts: { url: string, layout: ... }
            // The URL must be accessible.

            // For now, let's log. We'll refine this "Local File" problem in integration.
            console.log('Setting Wallpaper:', msg.url);

            // Mock call if strictly testing, real call if API exists
            chrome.wallpaper.setWallpaper({
                url: msg.url,
                layout: 'CENTER_CROPPED',
                filename: 'spice_wallpaper'
            }, (res) => {
                if (chrome.runtime.lastError) {
                    console.error('Wallpaper Error:', chrome.runtime.lastError);
                } else {
                    console.log('Wallpaper Set!');
                }
            });
        } else {
            console.log('chrome.wallpaper API not available (Not ChromeOS?)');
        }
    }
}

// --- Smart Clipper / Icon Animation ---

function startAnimation() {
    if (animationInterval) return;
    let frame = 0;
    animationInterval = setInterval(() => {
        frame = (frame + 1) % 2;
        // Toggle between two icons
        const iconPath = frame === 0 ? "/icon_128.png" : "/icon_anim_128.png";
        chrome.action.setIcon({ path: iconPath });
    }, 500);
}

function stopAnimation() {
    if (animationInterval) clearInterval(animationInterval);
    animationInterval = null;
    chrome.action.setIcon({ path: "/icon_128.png" });
}

// Monitor Tabs for Supported Sites
// REGEX_START
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
// REGEX_END

// Helper to check URL and update state
function checkAndStoreUrl(url) {
    if (!url) {
        stopAnimation();
        chrome.storage.local.remove("detected_source");
        return;
    }
    const isSupported = SUPPORTED_PATTERNS.some(regex => regex.test(url));
    if (isSupported) {
        startAnimation();
        chrome.storage.local.set({ "detected_source": url });
    } else {
        stopAnimation();
        chrome.storage.local.remove("detected_source");
    }
}

chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
    if (changeInfo.status === 'complete' && tab.url) {
        checkAndStoreUrl(tab.url);
    }
});

chrome.tabs.onActivated.addListener((activeInfo) => {
    chrome.tabs.get(activeInfo.tabId, (tab) => {
        if (chrome.runtime.lastError) return; // Tab might be closed
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

// Initial Connect
connect();

chrome.runtime.onMessage.addListener((request, sender, sendResponse) => {
    if (request.type === "GET_STATUS") {
        sendResponse({ connected: isConnected });
    }
});
