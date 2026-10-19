// Behaviour for the leads tables (results preview and public share page).
// Uses event delegation so it also works for tables htmx swaps in later.
(function () {
    function filterTable(input) {
        var panel = input.closest('[data-leads-panel]');
        if (!panel) return;

        var query = input.value.trim().toLowerCase();
        var rows = panel.querySelectorAll('.leads-table tbody tr');
        var shown = 0;

        rows.forEach(function (row) {
            var match = query === '' || row.textContent.toLowerCase().indexOf(query) !== -1;
            row.hidden = !match;
            if (match) shown++;
        });

        var count = panel.querySelector('[data-leads-count]');
        if (count) {
            var total = count.getAttribute('data-total');
            count.textContent = query === '' ? total + (total === '1' ? ' business' : ' businesses') : shown + ' of ' + total + ' shown';
        }

        var noMatch = panel.querySelector('[data-leads-nomatch]');
        if (noMatch) noMatch.hidden = shown !== 0 || query === '';
    }

    function copyText(text) {
        if (navigator.clipboard && window.isSecureContext) {
            return navigator.clipboard.writeText(text);
        }

        // Fallback for browsers without the async clipboard API.
        var area = document.createElement('textarea');
        area.value = text;
        area.setAttribute('readonly', '');
        area.style.position = 'fixed';
        area.style.opacity = '0';
        document.body.appendChild(area);
        area.select();
        try {
            document.execCommand('copy');
        } finally {
            document.body.removeChild(area);
        }

        return Promise.resolve();
    }

    document.addEventListener('input', function (event) {
        if (event.target.matches('[data-leads-filter]')) filterTable(event.target);
    });

    document.addEventListener('click', function (event) {
        var button = event.target.closest('[data-copy-link]');
        if (!button) return;

        var url = new URL(button.getAttribute('data-copy-link'), window.location.origin).href;
        var label = button.textContent;

        copyText(url).then(function () {
            button.textContent = 'Link copied';
        }, function () {
            window.prompt('Copy this link:', url);
        }).finally(function () {
            setTimeout(function () { button.textContent = label; }, 2000);
        });
    });

    window.closeResults = function () {
        var container = document.getElementById('map-modal-container');
        if (container && container.querySelector('#results-modal')) container.innerHTML = '';
    };

    document.addEventListener('keydown', function (event) {
        if (event.key === 'Escape') window.closeResults();
    });

    // Show dates in the viewer's time zone on the share page.
    document.addEventListener('DOMContentLoaded', function () {
        document.querySelectorAll('.share-page time[datetime]').forEach(function (el) {
            var d = new Date(el.getAttribute('datetime'));
            if (!isNaN(d)) el.textContent = d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
        });
    });
})();
