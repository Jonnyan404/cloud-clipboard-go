// ⚠️ 本文件由 build.py 从 shortcuts.json 自动生成，**别手改**。
// 要改这段逻辑：在 App 里改捷径 → 重新导出 shortcuts.json → 再跑一次 build.py。
//
// 下面这段与 shortcuts.json 里「接收最新」「接收指定ID」两处的 codeOnSuccess 逐字相同
// （verify.mjs 会断言那两处必须一致，所以只需要留一份）。

const profile = JSON.parse(response.body)
const type = profile.type

if (type == 'text') {
    const ClibboardText = profile.content;
    copyToClipboard(ClibboardText);
    showToast('已拷贝\n' + ClibboardText);

    const httpstr = httpString(ClibboardText);

    if (httpstr) {
        if (confirm('包含网址，是否打开')) {
            openUrl(httpstr[0]);
        }
    }
}
else if (profile.name && profile.size > 0) {
    // 下载是**第二次**请求：第一次请求 URL 上的 &auth= 不会跟着走，必须自己带上。
    // 漏掉它的表现很具体 —— 文本正常（内容内联在 JSON 里，不用再发请求），
    // 文件/图片在设了密码的房间里一律 401。
    // 不用传 room：服务端按文件自己记录的房间鉴权，客户端说了不算（也是防伪造）。
    const auth = getVariable("auth")
    const downloadUrl = getVariable("url") + "/file/" + profile.uuid + "/" + encodeURIComponent(profile.name)
        + (auth ? "?auth=" + encodeURIComponent(auth) : "")
    const inputPara = { 'downloadUrl': downloadUrl }
    showToast('文件名已拷贝，正在下载\n' + profile.name)
    copyToClipboard(profile.name)
    if (type == 'image' || isImageFile(profile.name)) {
        enqueueShortcut(/*[shortcut]*/"1e693964-ab59-4e9c-902b-6b94b90ff2f0"/*[/shortcut]*/, inputPara)
    } else {
        enqueueShortcut(/*[shortcut]*/"1e693964-ab59-4e9c-902b-6b94b90ff2f0"/*[/shortcut]*/, inputPara)
    }
}

function isImageFile(file) {
    const filename = file.toLowerCase();
    const list = [
        '.png',
        '.jpg',
        '.jpeg',
        '.gif',
        '.bmp',
        '.webp',
    ]
    let result = false
    list.forEach(element => {
        if (filename.endsWith(element)) {
            result = true
        }
    })
    return result
}

function httpString(s) {
    var reg = /(https?|http|ftp|file):\/\/[-A-Za-z0-9+&@#/%?=~_|!:,.;]+[-A-Za-z0-9+&@#/%=~_|]/g;
    s = s.match(reg);
    return (s)
}
