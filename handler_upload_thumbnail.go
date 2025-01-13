package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// parse to multipart and get data + metadata
	const maxMemory = 10 << 20
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse data", err)
		return
	}
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse from file", err)
		return
	}
	defer file.Close()

	// analyze metadata to get image type as contentType and split it to mainType and subType, if possible
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		respondWithError(w, http.StatusBadRequest, "Couldn't find valid Content-Type header", nil)
		return
	}
	parts := strings.Split(contentType, "/")
	if len(parts) != 2 {
		respondWithError(w, http.StatusBadRequest, "malformed Content-Type header", nil)
		return
	}
	mainType := parts[0]
	subType := parts[1]
	if mainType != "image" {
		respondWithError(w, http.StatusBadRequest, "Content Type doesn't match image data type - thumbnail has to be an image", nil)
		return
	}

	/*
		// get image data
		imageData, err := io.ReadAll(file)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Couldn't read image data", err)
			return
		}
	*/

	// fetch video meta data, if possible
	metaData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't find video with such id", err)
		return
	}
	if metaData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "access permitted", nil)
		return
	}

	/*
		// save thumbnail data to global map
		videoThumbnail := thumbnail{
			data:      imageData,
			mediaType: contentType,
		}
		videoThumbnails[videoID] = videoThumbnail
	*/

	/*
		//save thumbnail data to sqlite db
		imageDataBase64 := base64.StdEncoding.EncodeToString(imageData)
		dataURL := fmt.Sprintf("data:%s;base64;%s", contentType, imageDataBase64)
	*/

	// save thumbnail data to /assets
	assetName := fmt.Sprintf("%s.%s", videoIDString, subType)
	assetLocalURL := filepath.Join(cfg.assetsRoot, assetName)
	newAssetFile, err := os.Create(assetLocalURL)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create dest path to new asset file", err)
		return
	}
	_, err = io.Copy(newAssetFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't write content to new asset file at assets/<new_asset>", err)
	}

	// update database thumbnail_url of video at videoID
	// thumbnailUrl := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", cfg.port, videoIDString)
	assetURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetName)
	updatedVideo := database.Video{
		ID:           metaData.ID,
		CreatedAt:    metaData.CreatedAt,
		UpdatedAt:    time.Now(),
		ThumbnailURL: &assetURL,
		VideoURL:     metaData.VideoURL,
		CreateVideoParams: database.CreateVideoParams{
			Title:       metaData.Title,
			Description: metaData.Description,
			UserID:      metaData.UserID,
		},
	}
	err = cfg.db.UpdateVideo(updatedVideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video, although videoID had been found", err)
	}

	respondWithJSON(w, http.StatusOK, updatedVideo)
}
