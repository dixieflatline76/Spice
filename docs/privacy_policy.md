---
layout: default
title: Privacy Policy
---

# Privacy Policy

**Effective Date:** December 18, 2025

## Introduction

This Privacy Policy describes how the **Spice Wallpaper Manager** (the "Application") and the **Spice Wallpaper Manager Extension** (the "Extension") collect, use, and handle your information. We are committed to protecting your privacy and ensuring that your data remains under your control.

## Data Collection and Usage

### 1. No Personal Identifiable Information (PII)
We **do not** collect, store, or transmit any personally identifiable information (PII) such as your name, email address, physical address, or phone number.

### 2. Local-Only Operation (No Central Server)
Spice is a **100% client-side application**. We do not operate a central backend server.
- **Spice Wallpaper Manager (Desktop App):** Stores your wallpaper configuration, history, and image references locally on your computer's file system.
- **Spice Wallpaper Manager Extension:** Communicates **only** with the locally installed Desktop App via a local loopback connection (`127.0.0.1` / `localhost`).
- **No Remote Storage:** Since there is no central server, we have no technical means to collect, view, or store your data remotely. All your personal configurations and selected images stay on your machine.

### 3. Website Content and Web History (Extension Only)
The Extension requires permission to read the URL of your active browser tab ("activeTab" or host permissions) for the sole purpose of:
1.  **Detection:** Identifying if you are currently viewing a supported wallpaper collection page (e.g., on Wallhaven, Pexels, Unsplash, or Wikimedia Commons).
2.  **Functionality:** Enabling the "Add to Spice" button to transfer that specific collection URL to your local Desktop App.

**Crucially:**
- This URL checking occurs locally within your browser.
- URL data is **never** sent to us or any third-party analytics servers.
- URL data is transmitted to the Spice Desktop App on your machine only when you explicitly click the "Add to Spice" button.

### 4. Google Photos API Data (Photos Picker API)
Our Application uses the **Google Photos Picker API** to allow you to select your own photos for use as wallpapers.
- **Access:** We only access the specific media items that you explicitly select using the Google-provided picker dialog. We do not have access to your entire library.
- **Storage:** We store a local reference (product URL) for the selected images to allow the Application to fetch and apply them as wallpapers. This metadata is stored only on your local machine.
- **Usage:** Your Google Photos data is used solely to provide the wallpaper management functionality of Spice. 
- **Limited Use Disclosure:** Spice's use and transfer to any other app of information received from Google APIs will adhere to [Google API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy), including the Limited Use requirements.

### 5. Other Third-Party Services
The Application connects to other third-party wallpaper providers (Wallhaven, Pexels, Unsplash, Wikimedia Commons) to download images *at your request*.
- When you add a collection or fetch wallpapers, your IP address and request details are visible to these third-party providers as part of standard HTTP web traffic.
- We do not control these third parties and their data practices are governed by their respective privacy policies.

## Data Selling and Sharing
We **do not** sell, trade, or share your personal data or Google User Data with third parties. No data is used for advertising or marketing purposes.

## Data Security
We implement local security measures to protect your configuration and metadata. Since all data is stored locally, the security of your information also depends on the security of your personal computer.

## Open Source & Transparency
Spice is an open-source project. We believe in transparency and security by design.
- **Source Code:** You are welcome and encouraged to review our full source code on [GitHub](https://github.com/dixieflatline76/Spice).
- **Verifiable Builds:** All releases are built automatically using GitHub Actions transparency logs.
- **Integrity:** We provide SHA-256 hashcodes for all binary releases, ensuring that the software you run matches the source code and has not been tampered with.

## Changes to This Policy
We may update our Privacy Policy from time to time. We will notify you of any changes by posting the new Privacy Policy on this page.

## Contact Us
If you have any questions about this Privacy Policy, please contact us via our GitHub repository issues page: [https://github.com/dixieflatline76/Spice/issues](https://github.com/dixieflatline76/Spice/issues)
