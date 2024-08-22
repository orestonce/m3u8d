package m3u8d

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SpeedStatus struct {
	Locker    sync.Mutex
	IsRunning bool

	speedBeginTime  time.Time
	speedIdAlloc    uint32
	totalBlockCount int64
	doneBlockCount  int64
	downBlockMap    map[time.Time]downOneUnit
	doingBlockMap   map[uint32]doingOneUnit

	progressPercent  int
	progressBarTitle string
	ProgressBarShow  bool
	lastDrawProgress time.Time

	errMsg     string
	saveFileTo string
	isSkipped  bool
}

type downOneUnit struct {
	byteCount  int64
	blockCount int64
}

type doingOneUnit struct {
	startTime time.Time
	doneBytes int
}

func (this *SpeedStatus) clearStatusNoLock() {
	this.speedBeginTime = time.Time{}
	this.totalBlockCount = 0
	this.doneBlockCount = 0
	this.speedIdAlloc = 0
	this.downBlockMap = map[time.Time]downOneUnit{}
	this.doingBlockMap = map[uint32]doingOneUnit{}

	this.progressPercent = 0
	this.progressBarTitle = ""
	this.ProgressBarShow = false

	this.errMsg = ""
	this.saveFileTo = ""
	this.isSkipped = false
}

func (this *SpeedStatus) DrawProgressBar(total int, current int) {
	if total == 0 {
		return
	}
	proportion := float32(current) / float32(total)

	this.Locker.Lock()
	this.progressPercent = int(proportion * 100)
	title := this.progressBarTitle
	if this.ProgressBarShow {
		if this.lastDrawProgress.IsZero() || time.Now().Sub(this.lastDrawProgress).Milliseconds() > 100 {
			width := 50
			pos := int(proportion * float32(width))
			fmt.Printf(title+" %s%*s %6.2f%%\r", strings.Repeat("■", pos), width-pos, "", proportion*100)
		}
	}
	this.Locker.Unlock()
}

func (this *SpeedStatus) ResetTotalBlockCount(count int) {
	this.Locker.Lock()
	defer this.Locker.Unlock()

	this.totalBlockCount = int64(count)
	this.doneBlockCount = 0
}

func (this *SpeedStatus) SpeedAdd1Block(now time.Time, byteCount int) {
	this.Locker.Lock()

	unit := this.downBlockMap[now]
	unit.byteCount += int64(byteCount)
	unit.blockCount++
	this.downBlockMap[now] = unit
	this.doneBlockCount++

	cur := this.doneBlockCount
	total := this.totalBlockCount
	this.Locker.Unlock()

	if total > 0 {
		this.DrawProgressBar(int(total), int(cur))
	}
}

func (this *SpeedStatus) SpeedResetBytes() {
	this.Locker.Lock()
	defer this.Locker.Unlock()

	this.speedBeginTime = time.Now()
	this.totalBlockCount = 0
	this.doneBlockCount = 0
	this.downBlockMap = map[time.Time]downOneUnit{}
}

type SpeedInfo struct {
	BytePerSecond     int
	BytePerSecondText string
	RemainTime        int
	RemainTimeText    string
}

func (obj SpeedInfo) ToString() string {
	var text string
	if obj.BytePerSecondText == "" {
		return ""
	}
	text += "速度 " + obj.BytePerSecondText

	if obj.RemainTimeText != "" {
		text += ", 剩余时间 " + obj.RemainTimeText
	}
	return text
}

func (this *SpeedStatus) SpeedRecent5sGetAndUpdate() (speed SpeedInfo) {
	this.Locker.Lock()
	defer this.Locker.Unlock()

	now := time.Now()
	if this.speedBeginTime.IsZero() || now.Sub(this.speedBeginTime) < time.Second { // 1s以内, 暂时不计算速度
		return speed
	}

	const secondCount = 5

	expireTime := now.Add(-secondCount * time.Second) // 5秒内的
	var total downOneUnit

	for ct, v := range this.downBlockMap {
		if ct.Before(expireTime) {
			delete(this.downBlockMap, ct)
			continue
		}
		total.byteCount += v.byteCount
		total.blockCount += v.blockCount
	}
	realSecond := now.Sub(this.speedBeginTime).Seconds()
	if realSecond > secondCount {
		realSecond = secondCount
	}
	bytePerSecond := float64(total.byteCount) / realSecond
	bytePerSecond = this.addBytePerSecondWithDoing(now, bytePerSecond)

	speed.BytePerSecond = int(bytePerSecond)

	if bytePerSecond < 1024 {
		speed.BytePerSecondText = strconv.Itoa(int(bytePerSecond)) + " B/s"
	} else {
		bytePerSecond = bytePerSecond / 1024
		if bytePerSecond < 1024 {
			speed.BytePerSecondText = strconv.Itoa(int(bytePerSecond)) + " KB/s"
		} else {
			bytePerSecond = bytePerSecond / 1024
			speed.BytePerSecondText = strconv.FormatFloat(bytePerSecond, 'f', 2, 64) + " MB/s"
		}
	}

	if this.totalBlockCount > 0 && total.blockCount > 0 && this.doneBlockCount < this.totalBlockCount {
		secondPerBlock := realSecond / float64(total.blockCount)
		speed.RemainTime = int(secondPerBlock * float64(this.totalBlockCount-this.doneBlockCount))
		speed.RemainTimeText = fmt.Sprintf("%02d:%02d", speed.RemainTime/60, speed.RemainTime%60)
	}
	return speed
}

func (this *SpeedStatus) GetPercent() (percent int) {
	this.Locker.Lock()
	defer this.Locker.Unlock()

	return this.progressPercent
}

func (this *SpeedStatus) GetTitle() (title string) {
	this.Locker.Lock()
	defer this.Locker.Unlock()

	return this.progressBarTitle
}
func (this *SpeedStatus) SetProgressBarTitle(title string) {
	this.Locker.Lock()
	this.progressBarTitle = title
	this.Locker.Unlock()
}

func (this *SpeedStatus) SpeedReadAll(r io.Reader) (b []byte, err error) {
	b = make([]byte, 0, 512)
	var n int
	var unit doingOneUnit
	unit.startTime = time.Now()

	var id uint32
	this.Locker.Lock()
	this.speedIdAlloc++
	id = this.speedIdAlloc
	this.Locker.Unlock()

	for {
		n, err = r.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			this.Locker.Lock()
			delete(this.doingBlockMap, id)
			this.Locker.Unlock()

			return b, err
		}

		if len(b) == cap(b) {
			// Add more capacity (let append pick how much).
			b = append(b, 0)[:len(b)]
		}
		unit.doneBytes = len(b)
		this.Locker.Lock()
		if this.doingBlockMap == nil {
			this.doingBlockMap = map[uint32]doingOneUnit{}
		}
		this.doingBlockMap[id] = unit
		this.Locker.Unlock()
	}
}

func (this *SpeedStatus) addBytePerSecondWithDoing(now time.Time, bytePerSecond float64) float64 {
	sum := bytePerSecond

	for _, one := range this.doingBlockMap {
		second := now.Sub(one.startTime).Seconds()
		if second < 1 {
			continue
		}
		sum += float64(one.doneBytes) / second
	}
	return sum
}
