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
    const downloadUrl = getVariable("url") + "/file/" +profile.uuid+ "/" + encodeURIComponent(profile.name)
    const inputPara = { 'downloadUrl': downloadUrl }
    showToast('文件名已拷贝，正在下载\n' + profile.name)
    copyToClipboard(profile.name)
    if (type == 'image' || isImageFile(profile.name)) {
        enqueueShortcut("展示文件", inputPara)
    } else {
        enqueueShortcut("展示文件", inputPara)
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