$(function () {
     $("#spinner").show();
     $.getJSON('https://sadbox.org/geekhack/postsbyminute', function (data) {
        $("#spinner").hide();
        $("#postsByMinute").show();
        Highcharts.setOptions({
            global : {
                useUTC : false
            }
        });
        var currentime = new Date();
        $('#postsByMinute').highcharts({
            chart: {
                type: 'area'
            },
            title: {
                text: 'Activity in channel by time of day'
            },
            xAxis: {
                type: 'datetime',
                dateTimeLabelFormats: {
                    day: '%H:%M'
                },
                title: {
                    text: "Time of Post (UTC Offset: "+currentime.getTimezoneOffset()/60+")"
                }
            },
            yAxis: {
                title: {
                    text: 'Posts Per Minute'
                }
            },
            tooltip: {
                enabled: false
            },
            legend: {
                enabled: false
            },
            credits: {
                enabled: false
            },
            plotOptions: {
                area: {
                    pointStart: Date.UTC(0,0,0),
                    pointInterval: 60 * 1000,
                    enableMouseTracking: false,
                    marker: {
                        enabled: false,
                        states: {
                            hover: {
                                enabled: false
                            }
                        }
                    }
                }
            },
            series: [data]
        });
    });
});
