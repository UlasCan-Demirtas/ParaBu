const WebSocket = require('ws');
const ws = new WebSocket('ws://localhost:8080/ws');
ws.on('open', () => {
    ws.send(JSON.stringify({ action: "subscribe", topics: ["depth:BTC/USDT"] }));
});
ws.on('message', (data) => {
    console.log(data.toString());
});
