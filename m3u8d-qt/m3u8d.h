#pragma once

#include <cstdlib>
#include <string>
#include <vector>
#include <cstdint>
#include <map>
//Qt Creator 需要在xxx.pro 内部增加静态库的链接声明
//LIBS += -L$$PWD -lm3u8d-impl

struct RunDownload_Req{
	std::string M3u8Url;
	bool Insecure;
	std::string SaveDir;
	std::string FileName;
	int32_t SkipTsCountFromHead;
	std::string SetProxy;
	std::map<std::string, std::vector<std::string>> HeaderMap;
	bool SkipRemoveTs;
	bool ProgressBarShow;
	int32_t ThreadCount;
	RunDownload_Req(): Insecure(false),SkipTsCountFromHead(0),SkipRemoveTs(false),ProgressBarShow(false),ThreadCount(0){}
};
struct RunDownload_Resp{
	std::string ErrMsg;
	bool IsSkipped;
	bool IsCancel;
	std::string SaveFileTo;
	RunDownload_Resp(): IsSkipped(false),IsCancel(false){}
};
RunDownload_Resp RunDownload(RunDownload_Req in0);
void CloseOldEnv();
struct GetProgress_Resp{
	int32_t Percent;
	std::string Title;
	std::string StatusBar;
	GetProgress_Resp(): Percent(0){}
};
GetProgress_Resp GetProgress();
std::string GetWd();
struct ParseCurl_Resp{
	std::string ErrMsg;
	RunDownload_Req DownloadReq;
};
ParseCurl_Resp ParseCurlStr(std::string in0);
std::string RunDownload_Req_ToCurlStr(RunDownload_Req in0);
std::string GetFileNameFromUrl(std::string in0);
struct MergeTsDir_Resp{
	std::string ErrMsg;
	bool IsCancel;
	MergeTsDir_Resp(): IsCancel(false){}
};
MergeTsDir_Resp MergeTsDir(std::string in0, std::string in1);
void MergeStop();
struct MergeGetProgressPercent_Resp{
	int32_t Percent;
	std::string SpeedText;
	bool IsRunning;
	MergeGetProgressPercent_Resp(): Percent(0),IsRunning(false){}
};
MergeGetProgressPercent_Resp MergeGetProgressPercent();

#include <QObject>
#include <QVector>
#include <QThreadPool>
#include <QMutex>
#include <QMutexLocker>
#include <functional>

class RunOnUiThread : public QObject
{
    Q_OBJECT
public:
    explicit RunOnUiThread(QObject *parent = nullptr);
    virtual ~RunOnUiThread();

    void AddRunFnOn_OtherThread(std::function<void()> fn);
    // !!!注意,fn可能被调用,也可能由于RunOnUiThread被析构不被调用
    // 依赖于在fn里delete回收内存, 关闭文件等操作可能造成内存泄露
    void AddRunFnOn_UiThread(std::function<void ()> fn);
	bool Get_Done();
signals:
    void signal_newFn();
private slots:
    void slot_newFn();
private:
    bool m_done;
    QVector<std::function<void()>> m_funcList;
    QMutex m_Mutex;
    QThreadPool m_pool;
};

// Thanks: https://github.com/live-in-a-dream/Qt-Toast

#include <QString>
#include <QObject>

class QTimer;
class QLabel;
class QWidget;

namespace Ui {
class Toast;
}

class Toast : public QObject
{
    Q_OBJECT

public:
    explicit Toast(QObject *parent = nullptr);

    static Toast* Instance();
    //错误
    void SetError(const QString &text,const int & mestime = 3000);
    //成功
    void SetSuccess(const QString &text,const int & mestime = 3000);
    //警告
    void SetWaring(const QString &text,const int & mestime = 3000);
    //提示
    void SetTips(const QString &text,const int & mestime = 3000);
private slots:
    void onTimerStayOut();
private:
    void setText(const QString &color="FFFFFF",const QString &bgcolor = "000000",const int & mestime=3000,const QString &textconst="");
private:
    QWidget *m_myWidget;
    QLabel *m_label;
    QTimer *m_timer;
    Ui::Toast *ui;
};
