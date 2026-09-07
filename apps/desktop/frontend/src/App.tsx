import { useEffect, useState } from 'react';
import { Status } from '../wailsjs/go/main/App';
import { connectionMethodLabel } from './connection';
import './App.css';

function App() {
  const [platform, setPlatform] = useState('正在连接桌面后端…');
  const [error, setError] = useState('');
  useEffect(() => {
    let active = true;
    Status().then((status) => {
      if (active) setPlatform(status.platform + ' · ' + status.version);
    }).catch((cause: unknown) => {
      if (active) setError(String(cause));
    });
    return () => { active = false; };
  }, []);
  return (
    <main>
      <header><span className="wordmark">LinkSend</span><span className="preview">开发预览</span></header>
      <section aria-labelledby="title">
        <p className="eyebrow">设备间安全直传</p>
        <h1 id="title">从可信连接开始。</h1>
        <p className="description">通过自建服务器发现设备，以经过身份认证的加密连接传输文件。</p>
        <div className="status" role="status">
          <span className="status-dot" />
          <div><strong>核心服务已就绪</strong><p>身份、设备发现和直连传输核心已接入；当前窗口显示的是开发状态，尚未建立传输任务。</p></div>
        </div>
        {error && <p role="alert" className="error">无法连接桌面后端：{error}</p>}
        <dl>
          <div><dt>连接状态</dt><dd>未连接</dd></div>
          <div><dt>中继</dt><dd>{connectionMethodLabel('relay')}</dd></div>
          <div><dt>运行环境</dt><dd>{platform}</dd></div>
        </dl>
      </section>
      <footer>文件不会通过前端或信令服务器传输。网络与平台验收状态以项目进度文档为准。</footer>
    </main>
  );
}
export default App;
