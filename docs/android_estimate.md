# Spice Android Port: Effort Estimate

**Target Timeline: 2 Weeks (approx. 10 working days)**

This estimate is based on the **Hybrid Architecture** (Fyne App + Native Widget) and the **Interface Split** strategy (No Rewrite).

## Phase 1: The Foundation (Days 1-4)
*Goal: Decouple Fyne from Core Logic without breaking the Desktop App.*

*   **Day 1**: Refactor `pkg/provider` interfaces (`CoreProvider` vs `GUIProvider`) and abstract `pkg/config`.
*   **Day 2**: Pilot the "Split File" strategy on complex providers (`Unsplash`, `GooglePhotos`) and simple ones (`Wallhaven`).
*   **Day 3**: Complete the split for all 8 providers. Verify Desktop build passes.
*   **Day 4**: Update `Store` and `Pipeline` to use the new `CoreProvider` interface.

## Phase 2: The Android Bridge (Days 5-7)
*Goal: Get Go code running inside an Android Studio project.*

*   **Day 5**: Create `pkg/mobile` API (`WidgetHelper`). Run `gomobile bind` to generate the `.aar`.
*   **Day 6**: Initialize Android Studio project. Configure "Double Runtime" (separate process for widget).
*   **Day 7**: Implement the "Persistent Notification" (Kotlin) that calls Go to fetch images.

## Phase 3: The Widget & Polish (Days 8-10)
*Goal: A shipping-quality Android experience.*

*   **Day 8**: Implement the Home Screen Widget (Kotlin) and its bitmap rendering loop.
*   **Day 9**: Tune `Smart Fit` for mobile aspect ratios (testing on emulator/device).
*   **Day 10**: Testing, Permissions cleanup, and Release build.

## Risk Factors
1.  **JNI Overhead**: Passing large bitmaps from Go to Kotlin can be slow. We may need to optimize by passing file paths instead.
2.  **Process Lifecycle**: Ensuring the "Persistent Notification" isn't killed by Android's aggressive battery saver (Samsung/Pixel quirks).
