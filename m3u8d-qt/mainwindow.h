#ifndef MAINWINDOW_H
#define MAINWINDOW_H

#include <QMainWindow>
#include "m3u8d.h"

namespace Ui {
class MainWindow;
}

class MainWindow : public QMainWindow
{
    Q_OBJECT

public:
    explicit MainWindow(QWidget *parent = 0);
    ~MainWindow();

private slots:
    void on_pushButton_RunDownload_clicked();

    void on_pushButton_SaveDir_clicked();

    void on_pushButton_StopDownload_clicked();

    void on_pushButton_curlMode_clicked();

    void on_pushButton_returnDownload_clicked();

    void on_pushButton_gotoMergeTs_clicked();

    void on_pushButton_startMerge_clicked();

    void on_pushButton_stopMerge_clicked();

    void on_toolButton_selectMergeDir_clicked();

    void on_lineEdit_M3u8Url_editingFinished();

    void on_pushButton_TsTempDir_clicked();

    void on_pushButton_SetOutputMp4Name_clicked();

private:
    void updateDownloadUi(bool runing);
    void updateMergeUi(bool runing);

    void saveUiConfig();
    void loadUiConfig();
    QString getConfigFilePath();
private:
    Ui::MainWindow *ui;
    RunOnUiThread m_syncUi;
    QTimer *m_timer = nullptr;
    QTimer *m_saveConfigTimer = nullptr;
    std::map<std::string, std::vector<std::string>> m_HeaderMap;

    // QWidget interface
protected:
    virtual void closeEvent(QCloseEvent *event) override;
};

#endif // MAINWINDOW_H
