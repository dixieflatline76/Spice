package googlephotos

import "time"

// MediaItem represents a photo or video in Google Photos.
type MediaItem struct {
	ID            string        `json:"id"`
	ProductURL    string        `json:"productUrl"`
	BaseURL       string        `json:"baseUrl"`
	MimeType      string        `json:"mimeType"`
	MediaMetadata MediaMetadata `json:"mediaMetadata"`
	Filename      string        `json:"filename"`
}

// MediaMetadata contains metadata about the media item.
type MediaMetadata struct {
	CreationTime time.Time `json:"creationTime"`
	Width        string    `json:"width"`
	Height       string    `json:"height"`
	Photo        *Photo    `json:"photo,omitempty"`
	Video        *Video    `json:"video,omitempty"`
}

// Photo contains metadata specific to photos.
type Photo struct {
	CameraMake      string  `json:"cameraMake"`
	CameraModel     string  `json:"cameraModel"`
	FocalLength     float64 `json:"focalLength"`
	ApertureFNumber float64 `json:"apertureFNumber"`
	IsoEquivalent   int     `json:"isoEquivalent"`
	ExposureTime    string  `json:"exposureTime"`
}

// Video contains metadata specific to videos.
type Video struct {
	CameraMake  string  `json:"cameraMake"`
	CameraModel string  `json:"cameraModel"`
	Fps         float64 `json:"fps"`
	Status      string  `json:"status"`
}

// Album represents a Google Photos album.
type Album struct {
	ID                    string `json:"id"`
	Title                 string `json:"title"`
	ProductURL            string `json:"productUrl"`
	MediaItemsCount       string `json:"mediaItemsCount"`
	CoverPhotoBaseURL     string `json:"coverPhotoBaseUrl"`
	CoverPhotoMediaItemID string `json:"coverPhotoMediaItemId"`
}

// MediaItemsResponse represents the response from mediaItems.list/search.
type MediaItemsResponse struct {
	MediaItems    []MediaItem `json:"mediaItems"`
	NextPageToken string      `json:"nextPageToken"`
}

// AlbumsResponse represents the response from albums.list.
type AlbumsResponse struct {
	Albums        []Album `json:"albums"`
	NextPageToken string  `json:"nextPageToken"`
}

// SearchMediaItemsRequest represents the request body for mediaItems.search.
type SearchMediaItemsRequest struct {
	AlbumID   string   `json:"albumId,omitempty"`
	PageSize  int      `json:"pageSize,omitempty"`
	PageToken string   `json:"pageToken,omitempty"`
	Filters   *Filters `json:"filters,omitempty"`
}

// Filters defines search filters.
type Filters struct {
	ContentFilter   *ContentFilter   `json:"contentFilter,omitempty"`
	DateFilter      *DateFilter      `json:"dateFilter,omitempty"`
	MediaTypeFilter *MediaTypeFilter `json:"mediaTypeFilter,omitempty"`
}

// ContentFilter filters by content category.
type ContentFilter struct {
	IncludedContentCategories []string `json:"includedContentCategories"`
}

// DateFilter filters by date range.
type DateFilter struct {
	Ranges []DateRange `json:"ranges,omitempty"`
	Dates  []Date      `json:"dates,omitempty"`
}

// DateRange defines a start and end date.
type DateRange struct {
	StartDate Date `json:"startDate"`
	EndDate   Date `json:"endDate"`
}

// Date represents a simplified date.
type Date struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

// MediaTypeFilter filters by media type (PHOTO, VIDEO).
type MediaTypeFilter struct {
	MediaTypes []string `json:"mediaTypes"`
}

// PickerSessionRequest is the body for creating a session
type PickerSessionRequest struct {
	// Empty for now as Capabilities are not valid here
}

// PickerSessionResponse is the response from creating a session
type PickerSessionResponse struct {
	ID            string `json:"id"`
	PickerURI     string `json:"pickerUri"`
	MediaItemsSet bool   `json:"mediaItemsSet"`
	PollingConfig struct {
		PollInterval string `json:"pollInterval"` // e.g. "5s"
		Timeout      string `json:"timeout"`
	} `json:"pollingConfig"`
}

// PickerMediaItemResponse is the response from getting session items
type PickerMediaItemResponse struct {
	MediaItems    []PickerMediaItem `json:"mediaItems"`
	NextPageToken string            `json:"nextPageToken"`
}

type PickerMediaItem struct {
	ID         string `json:"id"`
	ProductUrl string `json:"productUrl"`
	MediaFile  struct {
		BaseURL  string `json:"baseUrl"`
		MimeType string `json:"mimeType"`
		Filename string `json:"filename"`
	} `json:"mediaFile"`
}
