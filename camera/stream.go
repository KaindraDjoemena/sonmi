package camera

import (
	"image"
	_ "image/jpeg"
	"mime"
	"mime/multipart"
	"net/http"
)

func StartStreamClient(url string, framePipe chan image.Image) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")

	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return err
	}

	boundary := params["boundary"]

	reader := multipart.NewReader(resp.Body, boundary)

	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}

		img, _, err := image.Decode(part)
		if err != nil {
			continue
		}

		framePipe <- img
	}

	return nil
}
