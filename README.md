# Spice
Spice is a desktop background manager for Windows that pays hommage to **[Variety](https://github.com/varietywalls/variety)**. Spice is written completely in Go and utilize Fyne for the limited UI it has such as the system tray icon and menu.

Spice is still in its infancy and has a lot of work ahead to even get close to the functionality and polish of Variety. It currently supports only Wallhaven but new wallpaper services will be added in the future.

Key features:
* wallhaven.cc support - API key support
* Multiple image queries support - add one query for scenery and another for people
* Windows system tray image controls - interactively go to the next image, previous image, or a random image
* Image cache daily refresh - never get bored with full image cache refreshing every midnight 
* Ondemand image download - Spice downloads the next page of image when you click next on the final image in the cache

Spice is tested on Windows 11 only at this time while it is still under heavy developement

Todos:
* Fix Windows Service support
* Add UI to remove the need to edit config.json
* Refactor to support other image/wallpaper services
