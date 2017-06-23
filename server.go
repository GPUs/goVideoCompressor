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
)

/*TODO  输入参数增加切片大小选项
        某个切片转码超时重新分配
        针对某个区间转码
*/

var ConnectId int
var AllConnects = make(map[int]net.Conn)
var OnConnect chan net.Conn
var remainMap = make(map[int]string)

func main() {
    mod := flag.String("mod", "", "s-split|c-convert|t-touch")
    filePath := flag.String("i", "", "file path")
    Args := flag.String("args", "", "ffmpeg args|segment_time|count_time")
    piece := flag.Int("p", 0, "piece number")
    flag.Parse()
    switch *mod {
    case "s":
        pieceSize := 100
        if len(*filePath) <= 0 {
            fmt.Printf("no input file\n")
            return
        }
        fileInfo, err:= os.Stat(*filePath)
        if err!=nil{
            fmt.Printf("[%s]not a file\n", *filePath)
            return
        }
        fileSize := fileInfo.Size()
        //单位转换成MB
        fileSize /= 1024*1024
        secondTime := GetSumTime(*filePath)
        tmp, err := strconv.Atoi(*Args)
        if err==nil{
            pieceSize = tmp
        }else {
            fmt.Printf("split input args[%s] error, ignore\n", *Args)
        }
        segmentTime := pieceSize * secondTime / int(fileSize)
        fmt.Printf("segment[%d]\n", segmentTime)
        segmentArgs := strconv.Itoa(segmentTime)
        fmt.Printf("split video....wait\n")
        b, path := SplitFile(*filePath, segmentArgs)
        fmt.Printf("[%s] [%d]\n", path, b)
        return
    case "c":
        if len(*filePath) <= 0 {
            fmt.Printf("no input file\n")
            return
        }
        if *piece < 0 {
            fmt.Printf("convert piece error[%d]", *piece)
            return
        }
        if 0 == *piece {
            *piece = fileCount(*filePath)
        }
        l, err := net.Listen("tcp", "0.0.0.0:8054")
        defer l.Close()
        if err != nil {
            fmt.Printf("Failure to listen: %s\n", err.Error())
            return
        }
        //去除路径中的最后一个斜杠
        fp := *filePath
        if (fp[len(fp)-1:]) == "/" {
            fp = fp[:len(fp)-1]
        }
        //print(fp + "\n")
        ftpDir := "/home/video/" + fp
        os.Mkdir(ftpDir, 0755)
        os.Chown(ftpDir, 2000, 2000)
        makeFileList(*piece, ftpDir)
        makeConcatScript(ftpDir)
        //go ReadStatus()
        go JobAlloc(fp, *piece, *Args)
        for {
            if c, err := l.Accept(); err == nil {
                go NewConnect(c)
            }
        }
    case "t":
        l, err := net.Listen("tcp", "0.0.0.0:8054")
        if err != nil {
            fmt.Printf("Failure to listen: %s\n", err.Error())
            return
        }
        defer l.Close()
        var waitTime int
        if len(*Args) > 0 {
            waitTime, err = strconv.Atoi(*Args)
            if err != nil {
                fmt.Printf("args error[%s]", *Args)
                return
            }
        } else {
            waitTime = 11
        }
        timeOut := make(chan bool, 1)
        go func(second int) {
            for ; second > 0; second-- {
                time.Sleep(time.Second)
                fmt.Printf("time[%d]\n", second)
            }
            timeOut <- true
        }(waitTime)
        go func() {
            for {
                if c, err := l.Accept(); err == nil {
                    AllConnects[ConnectId] = c
                    ConnectId++
                }
            }
        }()
        select {
        case <-timeOut:
            fmt.Printf("timeout\n")
        }
        fmt.Printf("####Online Client[%d]####\n", ConnectId)
        for key := 0; key < len(AllConnects); key++ {
            fmt.Printf("[%d]Client IP:port[%s]\n", key, AllConnects[key].RemoteAddr().String())
            AllConnects[key].Close()
        }
        return
    default:
        fmt.Printf("mod error[%s]\n", *mod)
        return
    }
    //b,path:=SplitFile("supa-159.mp4")
    //fmt.Printf("%d", b)

}

func JobAlloc(path string, num int, convertArgs string) {
    for i := 0; i < num; i++ {
        remainMap[i] = strconv.Itoa(i)
    }
    /*
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
    */
    OnConnect = make(chan net.Conn, 50)
Loop:
    for {
        select {
        case i := <-OnConnect:
            numStr := strconv.Itoa(num)
            _, err := i.Write([]byte(path + ";" + numStr + ";" + convertArgs))
            if err != nil {
                fmt.Printf("!!!send error[%s]\n", err.Error())
            } else {
                num--
                fmt.Printf("[%s]OnConnect send success [%s]\n", time.Now().Format("2006-01-02 15:04:05"), path+";"+numStr)
            }
            if num < 0 {
                break Loop
            }
        }
    }
}

//分割文件
func SplitFile(filePath string, segment_time string) (piece int, dir string) {
    //替换\为. 创建目录，分割出来的片段存到目录下
    dir = strings.Replace(filePath, "\\", ".", -1)
    dir = "12" + dir
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
    } else {
        fileNum --
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

//返回视频秒数
func GetSumTime(filePath string) (SumTime int) {
    cmd := exec.Command("ffmpeg", "-i", filePath)
    w := bytes.NewBuffer(nil)
    cmd.Stderr = w
    cmd.Run()
    //fmt.Printf("stderr: %s\n", string(w.Bytes()))
    //匹配出时间长度
    reg := regexp.MustCompile("Duration:(\\s+)(\\d+):(\\d+):(\\d+)")
    timeStr := []rune(reg.FindString(string(w.Bytes())))[len("Duration: "):]
    //计算秒数
    for key, value := range strings.Split(string(timeStr), ":") {
        i, _ := strconv.Atoi(value)
        switch key {
        case 0:
            SumTime += i * 3600
        case 1:
            SumTime += i * 60
        case 2:
            SumTime += i
        }
        //fmt.Printf("%s\n", value)
    }
    SumTime ++
    fmt.Printf("%d\n", SumTime)
    return
}

func NewConnect(c net.Conn) {
    fmt.Printf("new connect\n")
    go func() {
        for {
            data := make([]byte, 100)
            n, err := c.Read(data)
            if err == nil {
                Data := string(data[:n])
                switch {
                case Data[0:7] == "success":
                    pieceNum, _ := strconv.Atoi(strings.Split(Data, ";")[1])
                    fmt.Printf("##[%d]piece convert success\n", pieceNum)
                    c.Close()
                    delete(remainMap, pieceNum)
                    fmt.Printf("[%d]remain job %v\n", len(remainMap), remainMap)
                    return
                    //ConvertSuccess <- arr
                default:
                    continue
                }
            }
        }
    }()
    OnConnect <- c
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

func makeFileList(fileCount int, dirPath string) {
    var filePath string
    fileName := "filelist.txt"
    if dirPath[len(dirPath)-1:] == "/" {
        filePath = dirPath + fileName
    } else {
        filePath = dirPath + "/" + fileName
    }
    file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0644)
    if err != nil {
        fmt.Printf("makeFileList error[%v]\n", err)
        return
    }
    for i := 0; i <= fileCount; i++ {
        content := fmt.Sprintf("file '%d.mp4'\n", i)
        file.WriteString(content)
    }
    file.Close()
}

func makeConcatScript(dirPath string) {
    file := dirPath + "/" + "concat.sh"
    f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0755)
    if err != nil {
        fmt.Printf("makeConcatScript error[%v]\n", err)
    } else {
        f.WriteString("#!/bin/sh\nffmpeg -f concat -i filelist.txt -c copy output.mp4")
        f.Close()
    }
}
