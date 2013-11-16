$(function () {
     $.getJSON('http://sadbox.org:8080/geekhack/postsbyminute', function (data) {
        $('#postsByMinute').highcharts({
            chart: {
                type: 'area'
            },
            title: {
                text: 'Posts Broken Down to Minute Ranges'
            },
            xAxis: {
                labels: {
                    formatter: function() {
                        return this.value; // clean, unformatted number for year
                    }
                }
            },
            yAxis: {
                title: {
                    text: 'Total Posts'
                }
            },
            tooltip: {
                headerFormat: '<b>{point.y}</b> posts<br>',
                pointFormat: 'at {point.x} minute(s) after midnight UTC.'
            },
            legend: {
                enabled: false
            },
            plotOptions: {
                area: {
                    pointStart: 0,
                    marker: {
                        enabled: false,
                    }
                }
            },
            series: [data]
        });
    });
});
