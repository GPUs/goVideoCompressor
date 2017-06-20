package main

import (
    "net"
    "fmt"
    "time"
    "os"
    "strings"
    "os/exec"
    "bytes"
    "regexp"
    "strconv"
    "path/filepath"
    "flag"
    "sync"
)

var ConnectId int
var AllConnects = make(map[int]net.Conn)
var TickTime int = 10
var OnConnect chan int
var ConvertSuccess chan [2]int
var mapLock *sync.Mutex

func main() {
    mapLock = new(sync.Mutex)
    mod := flag.String("mod", "", "split|convert|count")
    filePath := flag.String("i", "", "file path")
    Args := flag.String("args", "", "ffmpeg args|segment_time|count_time")
    piece := flag.Int("p", 0, "piece number")
    flag.Parse()
    switch *mod {
    case "split":
        fmt.Printf("split video....wait\n")
        b, path := SplitFile(*filePath, *Args)
        fmt.Printf("[%s] [%d]\n", path, b-1)
        return
    case "convert":
        if *piece < 0 {
            fmt.Printf("convert piece error[%d]", *piece)
            return
        }
        l, err := net.Listen("tcp", "0.0.0.0:8054")
        defer l.Close()
        if err != nil {
            fmt.Printf("Failure to listen: %s\n", err.Error())
            return
        }
        ftpDir := "/home/video" + *filePath
        os.Mkdir(ftpDir, 0777)
        go ReadStatus()
        go JobAlloc(*filePath, *piece, *Args)
        for {
            if c, err := l.Accept(); err == nil {
                go NewConnect(c, ConnectId)
                ConnectId++
            }
        }
    case "count":
        l, err := net.Listen("tcp", "0.0.0.0:8054")
        defer l.Close()
        if err != nil {
            fmt.Printf("Failure to listen: %s\n", err.Error())
            return
        }
        waitTime, _ := strconv.Atoi(*Args)
        timeOut := make(chan bool, 1)
        go func(second int) {
            for ; second > 0; second-- {
                time.Sleep(time.Second)
                fmt.Printf("time[%d]\n", second)
                timeOut <- false
            }
            timeOut <- true
        }(waitTime)
    countLoop:
        for {
            go func() {
                if c, err := l.Accept(); err == nil {
                    mapLock.Lock()
                    AllConnects[ConnectId] = c
                    ConnectId++
                    mapLock.Unlock()
                }
            }()
            select {
            case ok := <-timeOut:
                if ok {
                    break countLoop
                }
            }
        }
        fmt.Printf("online client[%d]\n", ConnectId)
        for _, value := range AllConnects {
            value.Close()
        }
        return
    default:
        fmt.Printf("mod error[%s]\n", *mod)
        return
    }
    //b,path:=SplitFile("supa-159.mp4")
    //fmt.Printf("%d", b)

}

func countClient(c net.Conn, id int) {
    AllConnects[id] = c
}

func JobAlloc(path string, num int, convertArgs string) {
    remainMap := make(map[int]string)
    for i := 0; i < num; i++ {
        remainMap[i] = strconv.Itoa(i)
    }
    for _, c := range AllConnects {
        fmt.Printf("job[%d]to[%s]", num, c.RemoteAddr().String())
        numStr := strconv.Itoa(num)
        _, err := c.Write([]byte(path + ";" + numStr + ";" + convertArgs))
        if err != nil {
            fmt.Printf("send error[%s]", err.Error())
        } else {
            num--
            fmt.Printf("first alloc[%s]\n", path+";"+numStr)
        }
    }
    ConvertSuccess = make(chan [2]int, 10)
    OnConnect = make(chan int, 10)
Loop:
    for {
        select {
        case i := <-ConvertSuccess:
            AllConnects[i[0]].Close()
            delete(AllConnects, i[0])
            delete(remainMap, i[1])
            fmt.Printf("piece[%d] convert success\n", i[1])
            /*
            numStr := strconv.Itoa(num)
            _, err := AllConnects[i[0]].Write([]byte(path + ";" + numStr + ";" + convertArgs))
            if err != nil {
                fmt.Printf("!!!send error[%s]\n", err.Error())
            } else {
                num--
                fmt.Printf("ConvertSuccess send success[%d] [%s]\n", i, path+";"+numStr)
            }
            if num < 0 {
                break Loop
            }
            */
        case i := <-OnConnect:
            numStr := strconv.Itoa(num)
            _, err := AllConnects[i].Write([]byte(path + ";" + numStr + ";" + convertArgs))
            if err != nil {
                fmt.Printf("!!!send error[%s]\n", err.Error())
            } else {
                num--
                fmt.Printf("[%s]OnConnect send success[%d] [%s]\n", time.Now().Format("2006-01-02 15:04:05"), i, path+";"+numStr)
            }
            if num < 0 {
                break Loop
            }
        }
    }
}

func ReadStatus() {
    for {
        mapLock.Lock()
        for key, c := range AllConnects {
            data := make([]byte, 100)
            n, err := c.Read(data)
            if err == nil {
                Data := string(data[:n])
                switch {
                case Data[0:7] == "success":
                    var arr [2]int
                    arr[0] = key
                    pieceNum := strings.Split(Data, ";")[1]
                    arr[1], _ = strconv.Atoi(pieceNum)
                    fmt.Printf("[%d] convert success\n", key)
                    ConvertSuccess <- arr
                default:
                    fmt.Printf("heart[%d]\n", key)
                }
            }
        }
        mapLock.Unlock()
        time.Sleep(time.Second * 10)
    }
}

//分割文件
func SplitFile(filePath string, segment_time string) (piece int, dir string) {
    //替换\为. 创建目录，分割出来的片段存到目录下
    dir = strings.Replace(filePath, "\\", ".", -1)
    dir += "12"
    os.Mkdir(dir, 0755)
    /*timeSum := GetSumTime(filePath)
    //w := bytes.NewBuffer(nil)
    //TickTime 每块视频的大小
    for start:=0; start<timeSum; piece, start = piece+1, start+TickTime {
        fmt.Printf("start[%d] piece[%d]\n", start, piece)
        pieceName := strconv.Itoa(piece) + ".mp4"
        cmd := exec.Command("ffmpeg", "-i", filePath, "-ss", FormatTime(start),
            "-t", FormatTime(TickTime), "-codec", "copy", dir+"/"+pieceName)
        //cmd.Stderr = w
        cmd.Run()
        //fmt.Printf("%s\n", string(w.Bytes()))
    }*/
    cmd := exec.Command("ffmpeg", "-i", filePath, "-acodec", "copy", "-f", "segment", "-segment_time",
        segment_time, "-vcodec", "copy", "-reset_timestamps", "1", "-map", "0", dir+"/"+"%d.mp4")
    cmd.Run()
    piece = fileCount(dir)
    return
}

func fileCount(path string) (fileNum int) {
    err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
        if info == nil {
            return err
        }
        if info.IsDir() {
            return nil
        }
        //fmt.Println(path)
        fileNum++
        return nil
    })
    if err != nil {
        fmt.Printf("error [%v]\n", err)
    }
    return
}

func FormatTime(minute int) (formatString string) {
    strSlice := make([]string, 3)
    hh := minute / 60
    mm := minute % 60
    strSlice[0] = "0" + strconv.Itoa(hh)
    strSlice[1] = strconv.Itoa(mm)
    strSlice[2] = "00"
    if len(strSlice[1]) == 1 {
        strSlice[1] = "0" + strSlice[1]
    }
    formatString = strings.Join(strSlice, ":")
    return
}

//返回视频分钟数
func GetSumTime(filePath string) (SumTime int) {
    cmd := exec.Command("ffmpeg", "-i", filePath)
    w := bytes.NewBuffer(nil)
    cmd.Stderr = w
    cmd.Run()
    //fmt.Printf("stderr: %s\n", string(w.Bytes()))
    //匹配出时间长度
    reg := regexp.MustCompile("Duration:(\\s+)(\\d+):(\\d+):(\\d+)")
    timeStr := []rune(reg.FindString(string(w.Bytes())))[len("Duration: "):]
    //计算分钟数
    for key, value := range strings.Split(string(timeStr), ":") {
        i, _ := strconv.Atoi(value)
        switch key {
        case 0:
            SumTime += i * 60
        case 1:
            SumTime += i
        case 2:
            if i > 0 {
                SumTime ++
            }
        }
        fmt.Printf("%s\n", value)
    }
    fmt.Printf("%d\n", SumTime)
    return
}

func NewConnect(c net.Conn, i int) {
    fmt.Printf("new connect\n")
    mapLock.Lock()
    AllConnects[i] = c
    mapLock.Unlock()
    OnConnect <- i
}

func HeartBeat() {
    for {
        for key, c := range AllConnects {
            fmt.Printf("send heartbest to[%d]\n", key)
            _, err := c.Write([]byte("heart"))
            if err != nil {
                fmt.Printf("send heart error[%s] delete[%d]\n", err.Error(), key)
                delete(AllConnects, key)
            }
        }
        time.Sleep(time.Second * 3)
    }

}
