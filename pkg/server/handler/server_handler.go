package server_handler

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
	"github.com/gin-gonic/gin"
)

type ServerHandler interface {
	ServerOk(ctx *gin.Context)
	Stats(ctx *gin.Context)
}

type serverHandler struct {
	messageRepo message_repository.MessageRepository
	startTime   time.Time
}

// ServerOk implements ServerHandler.
func (s *serverHandler) ServerOk(ctx *gin.Context) {
	ctx.JSON(200, gin.H{
		"status": "ok",
	})
}

// Stats retorna métricas de sistema (runtime Go + host Linux) e de mensagens.
// [Athene] Usado pelo dashboard customizado (GET /dashboard). Auth: AuthAdmin.
func (s *serverHandler) Stats(ctx *gin.Context) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	const mb = 1024.0 * 1024.0
	system := gin.H{
		"goroutines":    runtime.NumGoroutine(),
		"numCpu":        runtime.NumCPU(),
		"goVersion":     runtime.Version(),
		"uptimeSeconds": int64(time.Since(s.startTime).Seconds()),
		"memAllocMB":    float64(mem.Alloc) / mb,
		"memSysMB":      float64(mem.Sys) / mb,
		"heapInuseMB":   float64(mem.HeapInuse) / mb,
		"numGC":         mem.NumGC,
	}

	if l1, l5, l15, ok := readLoadAvg(); ok {
		system["loadAvg1"] = l1
		system["loadAvg5"] = l5
		system["loadAvg15"] = l15
	}
	if totalKB, availKB, ok := readHostMem(); ok {
		system["hostMemTotalMB"] = totalKB / 1024.0
		system["hostMemAvailableMB"] = availKB / 1024.0
		if totalKB > 0 {
			system["hostMemUsedPct"] = (1 - availKB/totalKB) * 100
		}
	}

	messages := gin.H{"total": 0}
	if s.messageRepo != nil {
		if st, err := s.messageRepo.GetStats(); err == nil {
			messages = gin.H{
				"total":      st.Total,
				"byStatus":   st.ByStatus,
				"byDay":      st.ByDay,
				"topSources": st.TopSources,
			}
		}
	}

	ctx.JSON(200, gin.H{"system": system, "messages": messages})
}

// readLoadAvg lê /proc/loadavg (Linux). Retorna médias de 1/5/15 min.
func readLoadAvg() (float64, float64, float64, bool) {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, false
	}
	f := strings.Fields(string(b))
	if len(f) < 3 {
		return 0, 0, 0, false
	}
	l1, _ := strconv.ParseFloat(f[0], 64)
	l5, _ := strconv.ParseFloat(f[1], 64)
	l15, _ := strconv.ParseFloat(f[2], 64)
	return l1, l5, l15, true
}

// readHostMem lê /proc/meminfo (Linux). Retorna MemTotal e MemAvailable em kB.
func readHostMem() (float64, float64, bool) {
	fp, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	defer fp.Close()

	var total, avail float64
	sc := bufio.NewScanner(fp)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			total = parseMeminfoKB(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			avail = parseMeminfoKB(line)
		}
	}
	if total == 0 {
		return 0, 0, false
	}
	return total, avail, true
}

func parseMeminfoKB(line string) float64 {
	f := strings.Fields(line)
	if len(f) < 2 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[1], 64)
	return v
}

func NewServerHandler(messageRepo message_repository.MessageRepository) ServerHandler {
	return &serverHandler{messageRepo: messageRepo, startTime: time.Now()}
}
