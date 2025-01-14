# Spice - A Windows Desktop Background Manager

Spice is a work-in-progress desktop background manager for Windows, inspired by the legendarly linux wallpaper manager, [Variety](https://github.com/varietywalls/variety). Built entirely in Go and using Fyne for its minimalist UI (system tray icon and menu), Spice aims to provide a seamless and customizable wallpaper experience.

[Insert a screenshot or GIF of Spice in action here]

## Current Features

* **Wallhaven.cc support:** Fetch wallpapers from Wallhaven, with support for API keys.
* **Multiple image queries:** Define multiple queries to diversify your wallpaper collection (e.g., one for landscapes, another for abstract art).
* **System tray controls:** Easily navigate through your wallpaper cache with next, previous, and random image options.
* **Daily image cache refresh:** Keep your wallpapers fresh with automatic cache updates every midnight.
* **On-demand image download:** Spice downloads the next batch of images only when needed, optimizing performance.

## Future Plans

Spice is under active development! Here's what's on the roadmap:

1. **Dedicated UI:** A settings window for easier configuration, eliminating the need to manually edit config.json.
2. **Expanded wallpaper sources:** Support for more wallpaper services beyond Wallhaven (e.g., Unsplash, Pexels).
3. **Windows Service support:** Improved background service integration for better reliability.

## Current Status

Spice is currently in its early stages and primarily tested on Windows 11.

**Known Issues:**

* Occasional delays in wallpaper updates.
* Limited customization options in this version.

**Installation:**

1. Download the spice executable from [latest release](https://github.com/dixieflatline76/Spice/releases/latest).
2. Run executable and enjoy.

**Configuration:**

Edit the `config.json` file located at `%USERPROFILE%/.spice/config.json` to customize settings.

## Contributing

Contributions are welcome! Here's how you can get involved:

* **Report bugs:** Open an issue to report any problems you encounter.
* **Suggest features:** Open an issue to propose new features or improvements.
* **Contribute code:** Submit pull requests to contribute code changes.

## License

MIT License

Copyright (c) 2025 Karl Kwong

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
