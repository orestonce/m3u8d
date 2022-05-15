#pragma once

#include <string>
#include <vector>
#include <cstdint>
//Qt Creator 需要在xxx.pro 内部增加静态库的链接声明
//LIBS += -L$$PWD -lm3u8d-impl

struct RunDownload_Req{
	std::string M3u8Url;
	std::string HostType;
	bool Insecure;
	std::string SaveDir;
	std::string FileName;
	bool UserFfmpegMerge;
	int32_t SkipTsCountFromHead;
};
struct RunDownload_Resp{
	std::string ErrMsg;
	bool IsSkipped;
	std::string SaveFileTo;
};
RunDownload_Resp RunDownload(RunDownload_Req in0);
int32_t GetProgress();
std::string GetWd();

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
