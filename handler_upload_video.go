package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	// "log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// limit the upload limit to 1 GB (1 << 30)
	const maxBytes = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	// extract videoID from URL path parameters and parse it as a UUID
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// authenticate user to get userID
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

	// get video metadata from the database, if the user is not the video owner, return a http.StatusUnauthorized response
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't find video with such id", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "access permitted", nil)
		return
	}

	// parse the uploaded video file from the form data
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse from video file", err)
		return
	}
	defer file.Close()

	// validate the uploaded file to ensure it's an MP4 video
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		respondWithError(w, http.StatusBadRequest, "Couldn't find valid Content-Type header", nil)
		return
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse mime type", err)
		return
	}
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		respondWithError(w, http.StatusBadRequest, "malformed Content-Type header", nil)
		return
	}
	mainType := parts[0]
	subType := parts[1]
	if mainType != "video" && subType != "mp4" {
		respondWithError(w, http.StatusBadRequest, "Content Type doesn't match video data type - video upload has to be an video/mp4", nil)
		return
	}

	// save the uploaded file to a temporary file on disk
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temporary file on disk for video upload", err)
		return
	}
	tempFilePath := filepath.Join(os.TempDir(), tempFile.Name())
	defer os.Remove(tempFilePath)
	defer tempFile.Close()
	io.Copy(tempFile, file)

	// reset tempFile's file pointer to the beginning to enable reading
	tempFile.Seek(0, io.SeekStart)

	// put the object into S3 using PutObject - one needs bucket name, file key, file contents (body), content type
	randomBytes := make([]byte, 32)
	rand.Read(randomBytes)
	encodedName := base64.RawURLEncoding.EncodeToString(randomBytes)
	assetName := fmt.Sprintf("%s.%s", encodedName, subType)

	_, err = cfg.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &assetName,
		Body:        tempFile,
		ContentType: &mediaType,
	})

	// log.Printf("PutObjectOutput: %v", test)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload file to S3", err)
		return
	}

	// update the videoURL of the video record with S3 bucket and key
	s3URL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, assetName)
	cfg.db.UpdateVideo(database.Video{
		ID:           video.ID,
		CreatedAt:    video.CreatedAt,
		UpdatedAt:    time.Now(),
		ThumbnailURL: video.ThumbnailURL,
		VideoURL:     &s3URL,
		CreateVideoParams: database.CreateVideoParams{
			Title:       video.Title,
			Description: video.Description,
			UserID:      video.UserID,
		},
	})

}

func getVideoAspectRatio(filePath string) (string, error) {
	return "", nil
}
