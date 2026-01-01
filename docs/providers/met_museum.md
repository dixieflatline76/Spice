# The Met Museum Provider

> **Status**: Active (v1.6.0)
> **Type**: Online (API-based)
> **Focus**: Public Domain Art (Open Access)

## 1. Overview
The Met Museum provider is a "Featured Museum" implementation designed to serve as a digital gateway to The Metropolitan Museum of Art in New York. It prioritizes **quality over quantity** and **connection over utility**.

## 2. Design Philosophy ("Evangelist UI")

### 2.1 The "Romance" Copy
Unlike generic providers that list specs, The Met provider uses evocative language to inspiring users.
*   **Description**: "The crown jewel of New York City..."
*   **Goal**: To make the user feel the weight and prestige of the institution.

### 2.2 Collections as Tours
Collections are renamed to feel like curated experiences:
*   **Spice Melange** -> **"Director's Cut: Essential Masterpieces"**
*   **Asian Art** -> **"Arts of Asia"**

### 2.3 The "Plan a Visit" CTA
The standard "Map" button is relabeled to **"📍 Plan a Visit"**. This subtle psychological shift turns a passive reference tool into an active invitation to visit the physical location.

## 3. Technical Implementation

### 3.1 Hardcoded Fallback ("Ground Zero")
To guarantee a great first-run experience even offline:
*   **Mechanism**: `InitRemoteCollection` in `remote.go`.
*   **Fallback Chain**: Remote JSON -> Local Cache -> Embedded JSON -> **Hardcoded ID Slice**.
*   **Content**: A slice of 12 iconic landscapes (Van Gogh, Hokusai, etc.) is compiled directly into the binary.

### 3.2 Stability Measures
*   **Starvation Backoff**: If `TotalCount` doesn't increase during pagination (due to filtering), a 60s cooldown is enforced to prevent infinite loops.
*   **CAS Logic**: Atomic `fetchingInProgress` flag prevents race conditions from rapid UI clicks.

### 3.3 License Handling
*   **Open Access**: All images are filtered for `isPublicDomain=true`.
*   **UI**: The header displays a clickable "Open Access (CC0)" link to build trust.
