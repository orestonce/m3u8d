#pragma once

#include <cstdlib>
#include <string>
#include <vector>
#include <cstdint>
#include <map>
//Qt Creator 需要在xxx.pro 内部增加静态库的链接声明
//LIBS += -L$$PWD -lm3u8d-impl

struct StartDownload_Req{
	std::string M3u8Url;
	bool Insecure;
	std::string SaveDir;
	std::string FileName;
	std::string SkipTsExpr;
	std::string SetProxy;
	std::map<std::string, std::vector<std::string>> HeaderMap;
	bool SkipRemoveTs;
	bool ProgressBarShow;
	int32_t ThreadCount;
	bool SkipCacheCheck;
	bool SkipMergeTs;
	bool Skip_EXT_X_DISCONTINUITY;
	bool DebugLog;
	std::string TsTempDir;
	StartDownload_Req(): Insecure(false),SkipRemoveTs(false),ProgressBarShow(false),ThreadCount(0),SkipCacheCheck(false),SkipMergeTs(false),Skip_EXT_X_DISCONTINUITY(false),DebugLog(false){}
};
std::string StartDownload(StartDownload_Req in0);
void CloseOldEnv();
struct GetStatus_Resp{
	int32_t Percent;
	std::string Title;
	std::string StatusBar;
	bool IsDownloading;
	bool IsCancel;
	std::string ErrMsg;
	bool IsSkipped;
	std::string SaveFileTo;
	GetStatus_Resp(): Percent(0),IsDownloading(false),IsCancel(false),IsSkipped(false){}
};
GetStatus_Resp GetStatus();
GetStatus_Resp WaitDownloadFinish();
std::string GetWd();
struct ParseCurl_Resp{
	std::string ErrMsg;
	StartDownload_Req DownloadReq;
};
ParseCurl_Resp ParseCurlStr(std::string in0);
std::string RunDownload_Req_ToCurlStr(StartDownload_Req in0);
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
std::string FindUrlInStr(std::string in0);
std::string GetVersion();
#include <vector>
#include <functional>
#include <QMutex>
#include <QObject>
#include <QThreadPool>
#include <QMutexLocker>

class RunOnUiThread : public QObject
{
    Q_OBJECT
public:
    virtual ~RunOnUiThread();

    void AddRunFnOn_OtherThread(std::function<void()> fn);
    // !!!注意,fn可能被调用,也可能由于RunOnUiThread被析构不被调用
    // 依赖于在fn里delete回收内存, 关闭文件等操作可能造成内存泄露
    void AddRunFnOn_UiThread(std::function<void ()> fn);
    bool IsDone();
private slots:
    void slot_newFn();
private:
    bool m_done = false;
    std::vector<std::function<void()>> m_funcList;
    QMutex m_mutex;
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
