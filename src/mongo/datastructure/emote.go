package datastructure

import (
	"fmt"
	"io"
	"net/http"

	"gopkg.in/gographics/imagick.v3/imagick"

	"github.com/SevenTV/ServerGo/src/utils"
	log "github.com/sirupsen/logrus"
)

type emoteUtil struct{}

//
// Get size metadata of an emote
// (Width/Height)
//
func (*emoteUtil) AddSizeMetadata(emote *Emote) ([4]int16, [4]int16, error) {
	width := [4]int16{0, 0, 0, 0}
	height := [4]int16{0, 0, 0, 0}

	if MagickOK == false {
		InitMagickWand()
	}

	for i := int8(1); i <= 4; i++ {
		url := utils.GetCdnURL(emote.ID.Hex(), i)

		// Fetch emote data from the CDN
		res, err := http.Get(url)
		if err != nil {
			log.Errorf("utils, AddSizeMetadata(), err=%v", err)
			return width, height, err
		}

		// Decode the data
		// We'll use imagemagick to do this, as golang has no proper webp decoder at this time
		wand := imagick.NewMagickWand()
		b, err := io.ReadAll(res.Body)
		if err != nil {
			return width, height, err
		}

		if err = wand.ReadImageBlob(b); err != nil {
			return width, height, err
		}

		coalesce := wand.CoalesceImages()
		w := coalesce.GetImageWidth()
		h := coalesce.GetImageHeight()
		wand.Destroy()

		if err != nil {
			return width, height, err
		}

		width[i-1] = int16(w)
		height[i-1] = int16(h)
	}

	return width, height, nil
}

func (*emoteUtil) GetFilesMeta(fileDir string) [][]string {
	// Define sizes to be generated
	return [][]string{
		{fmt.Sprintf("%s/4x", fileDir), "4x", "384x128", "90"}, // Upscale: 3x * 1.685
		{fmt.Sprintf("%s/3x", fileDir), "3x", "228x76", "80"},  // Upscale: 2x * 1.585
		{fmt.Sprintf("%s/2x", fileDir), "2x", "144x48", "75"},  // Upscale: 1x * 1.5
		{fmt.Sprintf("%s/1x", fileDir), "1x", "96x32", "65"},
	}
}

var EmoteUtil emoteUtil

var MagickOK bool = false

func InitMagickWand() {
	imagick.Initialize()
	MagickOK = true
}