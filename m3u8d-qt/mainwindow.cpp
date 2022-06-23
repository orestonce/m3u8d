#include "mainwindow.h"
#include "ui_mainwindow.h"
#include "m3u8d.h"
#include <atomic>
#include <QFileDialog>

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
}

MainWindow::~MainWindow()
{
    CloseOldEnv();
    delete ui;
}

void MainWindow::on_pushButton_RunDownload_clicked()
{
    ui->lineEdit_M3u8Url->setEnabled(false);
    ui->lineEdit_SaveDir->setEnabled(false);
    ui->pushButton_SaveDir->setEnabled(false);
    ui->lineEdit_FileName->setEnabled(false);
    ui->lineEdit_SkipTsCountFromHead->setEnabled(false);
    ui->comboBox_HostType->setEnabled(false);
    ui->pushButton_RunDownload->setEnabled(false);
    ui->checkBox_Insecure->setEnabled(false);
    ui->progressBar->setValue(0);
    ui->lineEdit_SetProxy->setEnabled(false);
    ui->pushButton_StopDownload->setEnabled(true);

    m_syncUi.AddRunFnOn_OtherThread([this](){
        // isFinished被 other thread 和 ui thread共享
        std::shared_ptr<std::atomic_bool> isFinished = std::make_shared<std::atomic_bool>(false);

        while(isFinished->load() == false)
        {
            QThread::msleep(50);
            // 可能以下闭包在运行前, other thread已经退出了, 所以isFinished需要使用shared_ptr
            m_syncUi.AddRunFnOn_UiThread([this, isFinished](){
                GetProgress_Resp resp = GetProgress();
                ui->progressBar->setValue(resp.Percent);
                ui->label_progressBar->setText(QString::fromStdString(resp.Title));
                if (ui->pushButton_RunDownload->isEnabled()) {
                    isFinished->store(true);
                }
            });
        }
    });
    RunDownload_Req req;
    req.M3u8Url = ui->lineEdit_M3u8Url->text().toStdString();
    req.HostType = ui->comboBox_HostType->currentText().toStdString();
    req.Insecure = ui->checkBox_Insecure->isChecked();
    req.SaveDir = ui->lineEdit_SaveDir->text().toStdString();
    req.FileName = ui->lineEdit_FileName->text().toStdString();
    req.SkipTsCountFromHead = ui->lineEdit_SkipTsCountFromHead->text().toInt();
    req.SetProxy = ui->lineEdit_SetProxy->text().toStdString();

    m_syncUi.AddRunFnOn_OtherThread([req, this](){
        RunDownload_Resp resp = RunDownload(req);
        m_syncUi.AddRunFnOn_UiThread([req, this, resp](){
            ui->lineEdit_M3u8Url->setEnabled(true);
            ui->lineEdit_SaveDir->setEnabled(true);
            ui->pushButton_SaveDir->setEnabled(true);
            ui->lineEdit_FileName->setEnabled(true);
            ui->lineEdit_SkipTsCountFromHead->setEnabled(true);
            ui->comboBox_HostType->setEnabled(true);
            ui->pushButton_RunDownload->setEnabled(true);
            ui->checkBox_Insecure->setEnabled(true);
            ui->pushButton_RunDownload->setText("开始下载");
            ui->lineEdit_SetProxy->setEnabled(true);
            ui->pushButton_StopDownload->setEnabled(false);
            if (resp.IsCancel) {
                return;
            }

            if (!resp.ErrMsg.empty()) {
                Toast::Instance()->SetError(QString::fromStdString(resp.ErrMsg));
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
