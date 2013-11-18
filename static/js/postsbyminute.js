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
                formatter: function() {
                    return '<b>'+this.y.toPrecision(3)+'</b> posts per minute at <b>'+Highcharts.dateFormat('%H:%M', this.x)+'</b>'
                }
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
                    marker: {
                        enabled: false,
                        symbol: 'circle',
                        radius: 2,
                        states: {
                            hover: {
                                enabled: true
                            }
                        }
                    }
                }
            },
            series: [data]
        });
    });
});
