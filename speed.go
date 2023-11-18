package m3u8d

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SpeedStatus struct {
	speedBytesLocker sync.Mutex
	speedBeginTime   time.Time
	speedBytesMap    map[time.Time]int64

	progressLocker   sync.Mutex
	progressPercent  int
	progressBarTitle string
	progressBarShow  bool
}

func (this *SpeedStatus) DrawProgressBar(total int, current int) {
	if total == 0 {
		return
	}
	proportion := float32(current) / float32(total)

	this.progressLocker.Lock()
	this.progressPercent = int(proportion * 100)
	title := this.progressBarTitle
	if this.progressBarShow {
		width := 50
		pos := int(proportion * float32(width))
		fmt.Printf(title+" %s%*s %6.2f%%\r", strings.Repeat("■", pos), width-pos, "", proportion*100)
	}
	this.progressLocker.Unlock()
}

func (this *SpeedStatus) SpeedAddBytes(a int) {
	this.speedBytesLocker.Lock()
	defer this.speedBytesLocker.Unlock()

	now := time.Now()

	this.speedBytesMap[now] += int64(a)
}

func (this *SpeedStatus) SpeedResetBytes() {
	this.speedBytesLocker.Lock()
	defer this.speedBytesLocker.Unlock()

	this.speedBeginTime = time.Now()
	if this.speedBytesMap == nil {
		this.speedBytesMap = map[time.Time]int64{}
	}
	this.speedBytesMap = map[time.Time]int64{}
}

func (this *SpeedStatus) SpeedRecent5sGetAndUpdate() string {
	this.speedBytesLocker.Lock()
	defer this.speedBytesLocker.Unlock()

	now := time.Now()
	if this.speedBeginTime.IsZero() || now.Sub(this.speedBeginTime) < time.Second { // 1s以内, 暂时不计算速度
		return strconv.FormatBool(this.speedBeginTime.IsZero()) + " " + strconv.FormatBool(now.Sub(this.speedBeginTime) < time.Second)
	}

	const secondCount = 5

	expireTime := now.Add(-secondCount * time.Second)
	var total int64
	for ct, v := range this.speedBytesMap {
		if ct.Before(expireTime) {
			delete(this.speedBytesMap, ct)
			continue
		}
		total += v
	}
	realSecond := now.Sub(this.speedBeginTime).Seconds()
	if realSecond > secondCount {
		realSecond = secondCount
	}
	v := float64(total) / realSecond

	if v < 1024 {
		return strconv.Itoa(int(v)) + " B/s"
	}
	v = v / 1024
	if v < 1024 {
		return strconv.Itoa(int(v)) + " KB/s"
	}

	v = v / 1024
	return strconv.FormatFloat(v, 'f', 2, 64) + " MB/s"
}

func (this *SpeedStatus) GetPercent() (percent int) {
	this.progressLocker.Lock()
	defer this.progressLocker.Unlock()

	return this.progressPercent
}

func (this *SpeedStatus) GetTitle() (title string) {
	this.progressLocker.Lock()
	defer this.progressLocker.Unlock()

	return this.progressBarTitle
}
func (this *SpeedStatus) SetProgressBarTitle(title string) {
	this.progressLocker.Lock()
	this.progressBarTitle = title
	this.progressLocker.Unlock()
}
