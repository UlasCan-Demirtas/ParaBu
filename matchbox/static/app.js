document.addEventListener('DOMContentLoaded', () => {
    const connectionDot = document.getElementById('connection-dot');
    const connectionText = document.getElementById('connection-text');
    const asksContainer = document.getElementById('asks-container');
    const bidsContainer = document.getElementById('bids-container');
    const currentPriceEl = document.getElementById('current-price');
    const spreadValueEl = document.getElementById('spread-value');

    let maxVolume = 0;

    // Connect to WebSocket
    const ws = new WebSocket(`ws://${window.location.host}/ws`);

    ws.onopen = () => {
        connectionDot.classList.add('connected');
        connectionText.textContent = 'Connected (Live)';
    };

    ws.onclose = () => {
        connectionDot.classList.remove('connected');
        connectionText.textContent = 'Disconnected';
    };

    ws.onmessage = (event) => {
        const data = JSON.parse(event.data);
        renderOrderBook(data);
    };

    function formatNumber(num, decimals) {
        return parseFloat(num).toFixed(decimals);
    }

    function renderOrderBook(data) {
        // Calculate max volume for depth visualizer
        let localMax = 0;
        const processLevel = (level) => {
            const vol = parseFloat(level.quantity);
            if (vol > localMax) localMax = vol;
        };
        
        data.asks.forEach(processLevel);
        data.bids.forEach(processLevel);
        
        // Smoothly update max volume
        if (localMax > maxVolume) maxVolume = localMax;
        else maxVolume = (maxVolume * 0.95) + (localMax * 0.05);

        // Render Asks (reverse order so lowest price is at the bottom)
        asksContainer.innerHTML = '';
        
        // We want to show up to 10 asks. The server sends best asks first (lowest price first).
        // For display, we want the highest price at the top, lowest price at the bottom.
        const asksToShow = data.asks.slice(0, 10).reverse();
        
        // Fill empty rows if needed
        while (asksToShow.length < 10) {
            asksToShow.unshift({ price: '-', quantity: '-' });
        }

        let askTotal = 0;
        asksToShow.forEach(ask => {
            if (ask.price === '-') {
                asksContainer.appendChild(createEmptyRow());
                return;
            }
            const price = parseFloat(ask.price);
            const qty = parseFloat(ask.quantity);
            askTotal += (price * qty);
            
            const percentage = Math.min((qty / maxVolume) * 100, 100);
            asksContainer.appendChild(createRow(price, qty, askTotal, percentage, 'ask'));
        });

        // Update Spread and Current Price (using best bid or ask)
        const bestAsk = data.asks.length > 0 ? parseFloat(data.asks[0].price) : 0;
        const bestBid = data.bids.length > 0 ? parseFloat(data.bids[0].price) : 0;
        
        if (bestAsk > 0 || bestBid > 0) {
            const current = bestAsk > 0 ? bestAsk : bestBid;
            currentPriceEl.textContent = formatNumber(current, 2);
            currentPriceEl.style.color = bestBid > 0 ? 'var(--color-buy)' : 'var(--color-sell)';
            
            if (bestAsk > 0 && bestBid > 0) {
                const spread = bestAsk - bestBid;
                spreadValueEl.textContent = `Spread: ${formatNumber(spread, 2)}`;
            } else {
                spreadValueEl.textContent = `Spread: --`;
            }
        }

        // Render Bids (highest price at the top)
        bidsContainer.innerHTML = '';
        const bidsToShow = data.bids.slice(0, 10);
        
        let bidTotal = 0;
        bidsToShow.forEach(bid => {
            const price = parseFloat(bid.price);
            const qty = parseFloat(bid.quantity);
            bidTotal += (price * qty);
            
            const percentage = Math.min((qty / maxVolume) * 100, 100);
            bidsContainer.appendChild(createRow(price, qty, bidTotal, percentage, 'bid'));
        });

        // Fill empty rows if needed
        for (let i = bidsToShow.length; i < 10; i++) {
            bidsContainer.appendChild(createEmptyRow());
        }
    }

    function createRow(price, qty, total, percentage, type) {
        const div = document.createElement('div');
        div.className = 'row';
        
        div.innerHTML = `
            <div class="row-bg" style="width: ${percentage}%"></div>
            <span class="price">${formatNumber(price, 2)}</span>
            <span>${formatNumber(qty, 4)}</span>
            <span>${formatNumber(total, 2)}</span>
        `;
        return div;
    }

    function createEmptyRow() {
        const div = document.createElement('div');
        div.className = 'row';
        div.innerHTML = `
            <span class="price">--</span>
            <span>--</span>
            <span>--</span>
        `;
        return div;
    }
});
