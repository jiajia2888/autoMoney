package monitor

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"automoney/dao"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func SeedSamples(db *gorm.DB, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var input struct {
		Top20 []struct {
			URL          string `json:"url"`
			Title        string `json:"title_summary"`
			LikesDisplay string `json:"likes_display"`
		} `json:"top20"`
	}
	if err = json.Unmarshal(data, &input); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var a dao.BenchmarkAccount
		res := tx.Where("platform_account_id = ?", KnownID).First(&a)
		if res.Error == gorm.ErrRecordNotFound {
			a = dao.BenchmarkAccount{Platform: "dy", PlatformAccountID: KnownID, DouyinNumber: "80185543890", HomepageURL: "https://www.douyin.com/user/" + KnownID, DisplayName: "徐晶说", IntervalMinutes: 60, Threshold: 10000, MaxItems: 100, NotifyNew: true, NotifyThreshold: true, NextCheckAt: time.Now().UTC(), LastStatus: "sample"}
			if err := tx.Create(&a).Error; err != nil {
				return err
			}
		} else if res.Error != nil {
			return res.Error
		}
		observed := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
		for _, p := range input.Top20 {
			n, err := strconv.ParseFloat(strings.TrimSuffix(p.LikesDisplay, "万"), 64)
			if err != nil {
				return err
			}
			if strings.HasSuffix(p.LikesDisplay, "万") {
				n *= 10000
			}
			parts := strings.Split(p.URL, "/")
			v := dao.Video{AccountID: a.ID, PlatformID: parts[len(parts)-1], URL: p.URL, Title: p.Title, Likes: int64(n), LikesDisplay: p.LikesDisplay, LikesExact: false, FirstSeenAt: observed, LastSeenAt: observed, Source: "browser_sample_2026-11-08"}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&v).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
