#include "mainwindow.h"
#include "ui_mainwindow.h"
#include "m3u8d.h"
#include <atomic>
#include <QFileDialog>
#include <QIntValidator>
#include <QMessageBox>
#include <QFileDialog>
#include <QTimer>
#include <QDebug>
#include "curldialog.h"

MainWindow::MainWindow(QWidget *parent) :
    QMainWindow(parent),
    ui(new Ui::MainWindow),
    m_syncUi(parent)
{
    ui->setupUi(this);

    QIntValidator* vd = new QIntValidator(this);
    vd->setRange(0, 9999);
    ui->lineEdit_SkipTsCountFromHead->setValidator(vd);
    ui->lineEdit_SkipTsCountFromHead->setPlaceholderText("[0,9999]");
    ui->lineEdit_SaveDir->setPlaceholderText(QString::fromStdString(GetWd()));
    m_timer = new QTimer(this);
    connect(m_timer, &QTimer::timeout, [this](){
        //更新ui1
        {
            GetProgress_Resp resp = GetProgress();
            ui->progressBar->setValue(resp.Percent);
            ui->label_progressBar->setText(QString::fromStdString(resp.Title));
            if(!resp.StatusBar.empty())
                ui->statusBar->showMessage(QString::fromStdString(resp.StatusBar), 5*1000);
        }

        //更新ui2
        {
            auto resp = MergeGetProgressPercent();
            ui->progressBar_merge->setValue(resp.Percent);
            if(!resp.SpeedText.empty())
                ui->statusBar->showMessage(QString::fromStdString(resp.SpeedText), 5*1000);
        }
    });
    m_timer->start(50);
    this->updateDownloadUi(false);
    this->updateMergeUi(false);
}

MainWindow::~MainWindow()
{
    m_timer->stop();
    CloseOldEnv();
    delete ui;
}

void MainWindow::on_pushButton_RunDownload_clicked()
{
    if (ui->lineEdit_M3u8Url->isEnabled()==false) {
        return;
    }
    updateDownloadUi(true);

    RunDownload_Req req;
    req.M3u8Url = ui->lineEdit_M3u8Url->text().toStdString();
    req.Insecure = ui->checkBox_Insecure->isChecked();
    req.SaveDir = ui->lineEdit_SaveDir->text().toStdString();
    req.FileName = ui->lineEdit_FileName->text().toStdString();
    req.SkipTsCountFromHead = ui->lineEdit_SkipTsCountFromHead->text().toInt();
    req.SetProxy = ui->lineEdit_SetProxy->text().toStdString();
    req.HeaderMap = m_HeaderMap;
    req.SkipRemoveTs = ui->checkBox_SkipRemoveTs->isChecked();
    req.ThreadCount = ui->lineEdit_ThreadCount->text().toInt();

    m_syncUi.AddRunFnOn_OtherThread([req, this](){
        RunDownload_Resp resp = RunDownload(req);
        m_syncUi.AddRunFnOn_UiThread([req, this, resp](){
            this->updateDownloadUi(false);
            if (resp.IsCancel) {
                return;
            }

            if (!resp.ErrMsg.empty()) {
                QMessageBox::warning(this, "下载错误", QString::fromStdString(resp.ErrMsg));
                return;
            }
            if (resp.IsSkipped) {
                Toast::Instance()->SetSuccess("已经下载过了: " + QString::fromStdString(resp.SaveFileTo));
                return;
            }
            Toast::Instance()->SetSuccess("下载成功, 保存路径" + QString::fromStdString(resp.SaveFileTo));
        });
    });
}

void MainWindow::on_pushButton_SaveDir_clicked()
{
    QString dir = QFileDialog::getExistingDirectory(this);
    ui->lineEdit_SaveDir->setText(dir);
}

void MainWindow::on_pushButton_StopDownload_clicked()
{
    CloseOldEnv();
}

void MainWindow::on_pushButton_curlMode_clicked()
{
    RunDownload_Req req;
    req.M3u8Url = ui->lineEdit_M3u8Url->text().toStdString();
    req.Insecure = ui->checkBox_Insecure->isChecked();
    req.HeaderMap = m_HeaderMap;
    CurlDialog dlg(this);
    dlg.SetText(QString::fromStdString(RunDownload_Req_ToCurlStr(req)));
    dlg.show();
    if (dlg.exec() != QDialog::Accepted) {
        return;
    }
    ParseCurl_Resp resp= dlg.Resp;
    ui->lineEdit_M3u8Url->setText(QString::fromStdString(resp.DownloadReq.M3u8Url));
    ui->checkBox_Insecure->setChecked(resp.DownloadReq.Insecure);
    this->m_HeaderMap = resp.DownloadReq.HeaderMap;
}

void MainWindow::on_lineEdit_M3u8Url_textChanged(const QString &arg1)
{
    if (ui->lineEdit_FileName->text().isEmpty()==false) {
        return;
    }
    QString fileName = QString::fromStdString(GetFileNameFromUrl(arg1.toStdString()));
    if (fileName.isEmpty()) {
        return;
    }
    ui->lineEdit_FileName->setPlaceholderText(fileName);
}

void MainWindow::on_pushButton_returnDownload_clicked()
{
    ui->stackedWidget->setCurrentIndex(0);
}

void MainWindow::on_pushButton_gotoMergeTs_clicked()
{
    ui->stackedWidget->setCurrentIndex(1);
}

void MainWindow::on_pushButton_startMerge_clicked()
{
    QString fileName = ui->lineEdit_mergeFileName->text();
    if(fileName.isEmpty())
        fileName = ui->lineEdit_mergeFileName->placeholderText();
    QString dir = ui->lineEdit_mergeDir->text();

    this->updateMergeUi(true);

    m_syncUi.AddRunFnOn_OtherThread([=](){
        auto resp = MergeTsDir(dir.toStdString(), fileName.toStdString());

        m_syncUi.AddRunFnOn_UiThread([=](){
            this->updateMergeUi(false);
            if(resp.ErrMsg.empty())
                Toast::Instance()->SetSuccess("合并成功!");
            else if(!resp.IsCancel)
                QMessageBox::warning(this, "下载错误", QString::fromStdString(resp.ErrMsg));
        });
    });
}

void MainWindow::on_pushButton_stopMerge_clicked()
{
    MergeStop();
}

void MainWindow::on_toolButton_selectMergeDir_clicked()
{
    QString dir = QFileDialog::getExistingDirectory(this);
    if(dir.isEmpty())
        return;
    ui->lineEdit_mergeDir->setText(dir);
}

void MainWindow::updateDownloadUi(bool runing)
{
    ui->lineEdit_M3u8Url->setEnabled(!runing);
    ui->lineEdit_SaveDir->setEnabled(!runing);
    ui->pushButton_SaveDir->setEnabled(!runing);
    ui->lineEdit_FileName->setEnabled(!runing);
    ui->lineEdit_SkipTsCountFromHead->setEnabled(!runing);
    ui->pushButton_RunDownload->setEnabled(!runing);
    ui->checkBox_Insecure->setEnabled(!runing);
    if(runing == false)
        ui->pushButton_RunDownload->setText("开始下载");
    ui->lineEdit_SetProxy->setEnabled(!runing);
    ui->pushButton_StopDownload->setEnabled(runing);
    ui->pushButton_curlMode->setEnabled(!runing);
    ui->checkBox_SkipRemoveTs->setEnabled(!runing);
    ui->lineEdit_ThreadCount->setEnabled(!runing);

    if(runing == false)
        ui->progressBar->setValue(0);

    ui->pushButton_gotoMergeTs->setEnabled(!runing);
}

void MainWindow::updateMergeUi(bool runing)
{
    ui->lineEdit_mergeDir->setEnabled(!runing);
    ui->toolButton_selectMergeDir->setEnabled(!runing);
    ui->pushButton_stopMerge->setEnabled(runing);
    ui->pushButton_startMerge->setEnabled(!runing);
    ui->lineEdit_mergeFileName->setEnabled(!runing);
    ui->pushButton_returnDownload->setEnabled(!runing);
}
