import { corsHeaders } from './cors';

// 所有错误响应的唯一出口。字段与 Go 侧 writeError 保持一致，含义见那边的注释：
//   code    机器码（snake_case），给程序判断。发布后不要改。
//   error   英文人话，给日志和英文用户看。
//   message 中文人话，给人看。已分发的捷径、Android 快捷方式、前端都在展示它。
//
// 为什么必须统一：Apple 快捷指令的「获取URL内容」**不暴露 HTTP 状态码**，只能读响应体。
// 同一个状态码给出不同形状的响应体（有的带 message、有的只有中文 error、有的干脆是
// text/plain），等于要求每个客户端各写多套解析逻辑 —— 捷径曾把「文本超限」误报成
// 「服务器未确认保存，请检查部署地址及服务器状态」。
export function errorResponse(status, code, error, message) {
  return new Response(JSON.stringify({ code, error, message }), {
    status,
    headers: { 'Content-Type': 'application/json; charset=utf-8', ...corsHeaders },
  });
}
