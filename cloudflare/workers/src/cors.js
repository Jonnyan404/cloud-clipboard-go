export const corsHeaders = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type, Authorization, X-Requested-With, X-File-Name, X-File-Size, X-Room-Auth-Tokens',
  'Access-Control-Expose-Headers': 'Content-Length, Content-Type, Content-Disposition',
  'Access-Control-Max-Age': '86400'
};

export function handleCors(request) {
  return new Response(null, {
    status: 200,
    headers: corsHeaders
  });
}